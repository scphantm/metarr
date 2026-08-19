package hostinfo

// GPU reporting is best-effort and optional.
//
// There is no portable way to ask a machine about its GPU, and pulling in a
// vendor SDK would cost the agent its size and its cross-compilability — the
// agent is meant to drop onto a NAS as a single static binary. Shelling out to
// the vendor's own tool costs nothing when it is absent, which is the common
// case, and gives an accurate answer when it is there.

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"Metarr/internal/shared/agentproto"
)

// gpuProbeTimeout bounds the query. nvidia-smi on a busy or half-wedged driver
// can hang for a long time, and a heartbeat must not wait on it.
const gpuProbeTimeout = 2 * time.Second

type gpuProbe interface {
	probe(ctx context.Context) []agentproto.GPUTelemetry
}

// detectGPUProbe returns a probe for whatever this host can answer for, or nil
// when nothing can. Detection happens once at startup: a GPU does not appear
// while the process is running, and re-checking every heartbeat would mean an
// exec call every few seconds on the majority of hosts that have none.
func detectGPUProbe() gpuProbe {
	if path, err := exec.LookPath("nvidia-smi"); err == nil {
		return nvidiaProbe{path: path}
	}
	return nil
}

type nvidiaProbe struct {
	path string
}

func (p nvidiaProbe) probe(ctx context.Context) []agentproto.GPUTelemetry {
	ctx, cancel := context.WithTimeout(ctx, gpuProbeTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, p.path,
		"--query-gpu=name,utilization.gpu,memory.used,memory.total",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		// A driver that is installed but not currently answering reports no
		// GPU rather than failing the heartbeat around it.
		return nil
	}

	var gpus []agentproto.GPUTelemetry
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if gpu, ok := parseNvidiaLine(line); ok {
			gpus = append(gpus, gpu)
		}
	}
	return gpus
}

// parseNvidiaLine reads one CSV row of "name, util, used MiB, total MiB".
// nounits means the numbers arrive bare, and memory is always in MiB.
func parseNvidiaLine(line string) (agentproto.GPUTelemetry, bool) {
	fields := strings.Split(line, ",")
	if len(fields) < 4 {
		return agentproto.GPUTelemetry{}, false
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	const mib = 1024 * 1024
	utilization, _ := strconv.ParseFloat(fields[1], 64)
	usedMiB, _ := strconv.ParseUint(fields[2], 10, 64)
	totalMiB, _ := strconv.ParseUint(fields[3], 10, 64)

	return agentproto.GPUTelemetry{
		Name:             fields[0],
		UtilizationPct:   utilization,
		MemoryUsedBytes:  usedMiB * mib,
		MemoryTotalBytes: totalMiB * mib,
	}, true
}
