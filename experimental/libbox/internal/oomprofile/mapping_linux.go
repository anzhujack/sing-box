//go:build linux

package oomprofile

import (
	"os"
	"strconv"
	"strings"
)

func (b *profileBuilder) readMapping() {
	data, _ := os.ReadFile("/proc/self/maps")
	parseProcSelfMapsCompat(data, b.addMapping)
	if len(b.mem) == 0 {
		b.addMappingEntry(0, 0, 0, "", "", true)
	}
}

func parseProcSelfMapsCompat(data []byte, addMapping func(lo uint64, hi uint64, offset uint64, file string, buildID string)) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		rangeParts := strings.SplitN(fields[0], "-", 2)
		if len(rangeParts) != 2 {
			continue
		}
		lo, err1 := strconv.ParseUint(rangeParts[0], 16, 64)
		hi, err2 := strconv.ParseUint(rangeParts[1], 16, 64)
		off, err3 := strconv.ParseUint(fields[2], 16, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		file := ""
		if len(fields) >= 6 {
			file = strings.Join(fields[5:], " ")
		}
		addMapping(lo, hi, off, file, "")
	}
}
