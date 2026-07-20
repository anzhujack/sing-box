package assetdl

import (
	"context"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/service"
)

// ResolveTransport converts the current http_client option surface (or the
// deprecated download_detour fallback) into a transport owned by the shared
// HTTP client manager. The caller must not close the returned transport.
func ResolveTransport(
	ctx context.Context,
	logger log.ContextLogger,
	options *option.HTTPClientOptions,
	downloadDetour string,
) (adapter.HTTPTransport, string, error) {
	manager := service.FromContext[adapter.HTTPClientManager](ctx)
	configuredClient := options != nil && !options.IsEmpty()
	if configuredClient && downloadDetour != "" {
		return nil, "", E.New("http_client conflicts with deprecated download_detour")
	}
	if manager == nil {
		if configuredClient || downloadDetour != "" {
			return nil, "", E.New("HTTP client manager is unavailable")
		}
		return nil, "direct", nil
	}
	if configuredClient {
		transport, err := manager.ResolveTransport(ctx, logger, *options)
		if err != nil {
			return nil, "", err
		}
		name := "inline"
		if options.Tag != "" {
			name = options.Tag
		}
		return transport, name, nil
	}
	if downloadDetour != "" {
		transport, err := manager.ResolveTransport(ctx, logger, option.HTTPClientOptions{
			DialerOptions: option.DialerOptions{
				Detour: downloadDetour,
			},
			DisableEmptyDirectCheck: true,
		})
		if err != nil {
			return nil, "", err
		}
		return transport, downloadDetour, nil
	}
	transport := manager.DefaultTransport()
	if transport == nil {
		return nil, "direct", nil
	}
	return transport, "default", nil
}
