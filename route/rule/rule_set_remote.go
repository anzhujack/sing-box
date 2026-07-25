package rule

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/hash"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"
	"github.com/sagernet/sing/service/pause"
)

const (
	ruleSetInitialRetryInterval = 60 * time.Second
	ruleSetFetchTimeout         = 60 * time.Second
	ruleSetMaxResponseBytes     = 50 * 1024 * 1024
)

var _ adapter.RuleSet = (*RemoteRuleSet)(nil)

type ruleSetFetchAttempt struct {
	done chan struct{}
	err  error
}

type RemoteRuleSet struct {
	abstractRuleSet
	cancel               context.CancelFunc
	outbound             adapter.OutboundManager
	url                  string
	options              option.RemoteRuleSet
	updateInterval       time.Duration
	initialRetryDeadline time.Time
	httpClient           *http.Client
	hash                 hash.HashType
	lastEtag             string
	cacheFile            adapter.CacheFile
	pauseManager         pause.Manager
	fetchMu              sync.Mutex
	fetchAttempt         *ruleSetFetchAttempt
}

func NewRemoteRuleSet(ctx context.Context, logger logger.ContextLogger, tag string, options option.RuleSet) (*RemoteRuleSet, error) {
	ctx, cancel := context.WithCancel(ctx)
	var path string
	if options.Path != "" {
		path = filemanager.BasePath(ctx, strings.ReplaceAll(options.Path, C.RuleSetTagPlaceholder, tag))
		path, _ = filepath.Abs(path)
	}
	var updateInterval time.Duration
	if options.RemoteOptions.UpdateInterval > 0 {
		updateInterval = time.Duration(options.RemoteOptions.UpdateInterval)
	} else {
		updateInterval = 24 * time.Hour
	}
	return &RemoteRuleSet{
		abstractRuleSet: abstractRuleSet{
			ctx:    ctx,
			logger: logger,
			tag:    tag,
			sType:  options.Type,
			path:   path,
			format: options.Format,
		},
		outbound:       service.FromContext[adapter.OutboundManager](ctx),
		url:            strings.ReplaceAll(options.RemoteOptions.URL, C.RuleSetTagPlaceholder, tag),
		cancel:         cancel,
		options:        options.RemoteOptions,
		updateInterval: updateInterval,
		pauseManager:   service.FromContext[pause.Manager](ctx),
	}, nil
}

func (s *RemoteRuleSet) String() string {
	return strings.Join(F.MapToString(s.rules), " ")
}

func (s *RemoteRuleSet) StartContext(ctx context.Context, startContext *adapter.HTTPStartContext) error {
	s.cacheFile = service.FromContext[adapter.CacheFile](s.ctx)
	transport, err := s.resolveTransport()
	if err != nil {
		return E.Cause(err, "create rule-set http client")
	}
	startContext.Register(transport)
	s.httpClient = &http.Client{Transport: transport}
	if err = s.loadCacheFile(); err != nil {
		s.logger.Warn(E.Cause(err, "restore cached rule-set, ignoring cache for this run and refetching"))
		s.hash = hash.HashType{}
		s.lastEtag = ""
		s.setUpdatedTime(time.Time{})
	}
	if s.UpdatedTime().IsZero() {
		err = s.fetch(ctx, true)
		if err != nil {
			s.logger.Warn("initial rule-set ", s.tag, " fetch failed: ", err, " — starting empty and retrying in ", ruleSetInitialRetryInterval)
		}
	}
	return nil
}

func (s *RemoteRuleSet) recordFetchCompletion(err error, completedAt time.Time) {
	s.access.Lock()
	if err == nil {
		s.initialRetryDeadline = time.Time{}
	} else if s.lastUpdated.IsZero() {
		s.initialRetryDeadline = completedAt.Add(ruleSetInitialRetryInterval)
	}
	s.access.Unlock()
}

func (s *RemoteRuleSet) getInitialRetryDeadline() time.Time {
	s.access.RLock()
	defer s.access.RUnlock()
	return s.initialRetryDeadline
}

func (s *RemoteRuleSet) initialUpdateDelay(now time.Time) time.Duration {
	s.access.RLock()
	lastUpdated := s.lastUpdated
	deadline := s.initialRetryDeadline
	s.access.RUnlock()
	if !lastUpdated.IsZero() {
		return initialRuleSetUpdateDelay(lastUpdated, s.updateInterval, now)
	}
	if deadline.IsZero() {
		return ruleSetInitialRetryInterval
	}
	wait := deadline.Sub(now)
	if wait < 0 {
		return 0
	}
	return wait
}

func (s *RemoteRuleSet) update(ctx context.Context) bool {
	ctx = log.ContextWithNewID(ctx)
	err := s.fetch(ctx, false)
	if err != nil {
		s.logger.ErrorContext(ctx, "fetch rule-set ", s.tag, ": ", err)
		return false
	}
	s.Cleanup()
	return true
}

func (s *RemoteRuleSet) Update(ctx context.Context) error {
	err := s.fetch(log.ContextWithNewID(ctx), false)
	if err != nil {
		return err
	} else {
		s.Cleanup()
	}
	return nil
}

func (s *RemoteRuleSet) fetch(ctx context.Context, isStart bool) error {
	s.fetchMu.Lock()
	if attempt := s.fetchAttempt; attempt != nil {
		s.fetchMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-attempt.done:
			return attempt.err
		}
	}
	attempt := &ruleSetFetchAttempt{done: make(chan struct{})}
	s.fetchAttempt = attempt
	s.fetchMu.Unlock()

	err := s.fetchOnce(ctx, isStart)
	completedAt := time.Now()
	s.fetchMu.Lock()
	s.recordFetchCompletion(err, completedAt)
	attempt.err = err
	s.fetchAttempt = nil
	close(attempt.done)
	s.fetchMu.Unlock()
	return err
}

func (s *RemoteRuleSet) fetchOnce(ctx context.Context, isStart bool) error {
	s.logger.DebugContext(ctx, "updating rule-set ", s.tag, " from URL: ", s.url)
	requestCtx, cancel := context.WithTimeout(ctx, ruleSetFetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, s.url, nil)
	if err != nil {
		return err
	}
	if s.lastEtag != "" {
		request.Header.Set("If-None-Match", s.lastEtag)
	}
	if !isStart {
		defer s.httpClient.CloseIdleConnections()
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		lastUpdated := time.Now()
		s.setUpdatedTime(lastUpdated)
		if s.cacheFile != nil {
			if savedRuleSet := s.cacheFile.LoadRuleSet(s.tag); savedRuleSet != nil {
				savedRuleSet.LastUpdated = lastUpdated
				if err = s.cacheFile.SaveRuleSet(s.tag, savedRuleSet); err != nil {
					s.logger.Error("save rule-set updated time: ", err)
				}
			}
		}
		s.logger.InfoContext(ctx, "update rule-set ", s.tag, ": not modified")
		return nil
	default:
		return E.New("unexpected status: ", response.Status)
	}
	content, err := readRuleSetResponse(response.Body, ruleSetMaxResponseBytes)
	if err != nil {
		return E.Cause(err, "read rule-set ", s.tag)
	}
	err = s.loadBytes(content, s)
	if err != nil {
		return err
	}
	eTagHeader := response.Header.Get("Etag")
	if eTagHeader != "" {
		s.lastEtag = eTagHeader
	}
	lastUpdated := time.Now()
	s.setUpdatedTime(lastUpdated)
	if s.path != "" {
		if err = s.saveCacheFile(content); err != nil {
			return E.Cause(err, "save rule-set cache file")
		}
	}
	if s.cacheFile != nil {
		savedRuleSet := &adapter.SavedBinary{
			LastUpdated: lastUpdated,
			LastEtag:    s.lastEtag,
		}
		if s.path != "" {
			savedRuleSet.Hash = s.hash
		} else {
			savedRuleSet.Content = content
		}
		if err = s.cacheFile.SaveRuleSet(s.tag, savedRuleSet); err != nil {
			s.logger.Error("save rule-set cache: ", err)
		}
	}
	s.logger.InfoContext(ctx, "updated rule-set ", s.tag)
	return nil
}

func readRuleSetResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxBytes {
		return nil, E.New("response exceeds ", maxBytes, " bytes")
	}
	if len(content) == 0 {
		return nil, E.New("empty response")
	}
	return content, nil
}

func (s *RemoteRuleSet) resolveTransport() (adapter.HTTPTransport, error) {
	httpClientManager := service.FromContext[adapter.HTTPClientManager](s.ctx)
	if s.options.HTTPClient != nil && !s.options.HTTPClient.IsEmpty() {
		if s.options.DownloadDetour != "" { //nolint:staticcheck
			return nil, E.New("http_client is conflict with deprecated download_detour field")
		}
		return httpClientManager.ResolveTransport(s.ctx, s.logger, *s.options.HTTPClient)
	}
	if s.options.DownloadDetour != "" { //nolint:staticcheck
		deprecated.Report(s.ctx, deprecated.OptionLegacyRuleSetDownloadDetour)
		return httpClientManager.ResolveTransport(s.ctx, s.logger, option.HTTPClientOptions{
			DialerOptions: option.DialerOptions{
				Detour: s.options.DownloadDetour, //nolint:staticcheck
			},
			DisableEmptyDirectCheck: true,
		})
	}
	defaultTransport := httpClientManager.DefaultTransport()
	if defaultTransport == nil {
		return nil, E.New("default http client transport is not initialized")
	}
	return defaultTransport, nil
}

func (s *RemoteRuleSet) loadCacheFile() error {
	var content []byte
	var lastUpdated time.Time
	var lastEtag string
	var savedSet *adapter.SavedBinary
	if s.cacheFile != nil {
		if savedSet = s.cacheFile.LoadRuleSet(s.tag); savedSet != nil {
			s.hash = savedSet.Hash
		}
	}
	if s.path != "" {
		exists, err := pathExists(s.ctx, s.path)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		file, err := filemanager.Open(s.ctx, s.path)
		if err != nil {
			return err
		}
		content, err = io.ReadAll(file)
		if err != nil {
			file.Close()
			return err
		}
		info, err := file.Stat()
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if savedSet != nil {
			if !s.hash.Equal(hash.MakeHash(content)) {
				return E.New("load rule-set cache file failed: validation failed")
			}
			lastUpdated = savedSet.LastUpdated
			lastEtag = savedSet.LastEtag
		} else {
			lastUpdated = info.ModTime()
		}
	} else if savedSet != nil && len(savedSet.Content) > 0 {
		content = savedSet.Content
		lastUpdated = savedSet.LastUpdated
		lastEtag = savedSet.LastEtag
	} else {
		return nil
	}
	if err := s.loadBytes(content, s); err != nil {
		return err
	}
	s.setUpdatedTime(lastUpdated)
	s.lastEtag = lastEtag
	return nil
}

func pathExists(ctx context.Context, path string) (bool, error) {
	info, err := filemanager.Stat(ctx, path)
	if err == nil {
		if info.IsDir() {
			return false, E.New("rule_set path is a directory: ", path)
		}
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *RemoteRuleSet) saveCacheFile(content []byte) error {
	dir := filepath.Dir(s.path)
	err := filemanager.MkdirAll(s.ctx, dir, 0o755)
	if err != nil {
		return err
	}
	err = filemanager.WriteFile(s.ctx, s.path, content, 0o666)
	if err != nil {
		return err
	}
	s.hash = hash.MakeHash(content)
	return nil
}

func (s *RemoteRuleSet) Close() error {
	s.rules = nil
	s.cancel()
	return nil
}
