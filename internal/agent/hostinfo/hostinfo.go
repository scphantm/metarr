// Package hostinfo describes the machine an agent is running on: who it is,
// where it is, and what it is currently doing.
//
// Everything here degrades rather than fails. A host that cannot report its
// CPU, or has no GPU, or sits behind an interface the agent cannot enumerate,
// still has to heartbeat — losing telemetry should never look like losing the
// agent, because those two states mean very different things to whoever is
// looking at the screen.
package hostinfo

import (
	"context"
	"net"
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/agentproto"
)

// Identity gathers the facts about this host that do not change while the
// process runs. Fields that cannot be determined are left empty rather than
// guessed.
func Identity(slug, instanceID, version string, started time.Time) *agentproto.AgentIdentity {
	identity := &agentproto.AgentIdentity{
		Slug:       slug,
		InstanceId: instanceID,
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Version:    version,
		Started:    timestamppb.New(started),
		// Getuid returns -1 on Windows, which has no uid at all. Reporting
		// that verbatim is honest: the UI shows the account name there, which
		// is the thing that decides what the service can read either way.
		Uid: int32(os.Getuid()),
	}

	if hostname, err := os.Hostname(); err == nil {
		identity.Hostname = hostname
	}

	// user.Current works on every platform, unlike looking up a numeric uid.
	if account, err := user.Current(); err == nil {
		identity.Username = account.Username
	}

	identity.Ip = primaryIP()

	return identity
}

// primaryIP returns the host's main non-loopback address.
//
// It asks the routing table which local address would be used to reach the
// outside world, rather than picking the first interface that looks plausible.
// On a machine with several interfaces — a NAS with a management port and a
// storage port, say — the first one enumerated is frequently the wrong answer.
// No packet is sent: a UDP "connection" only resolves the route.
func primaryIP() string {
	conn, err := net.Dial("udp", "203.0.113.1:80")
	if err == nil {
		defer func() { _ = conn.Close() }()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}

	// No route out is normal on an isolated host, so fall back to the first
	// non-loopback address rather than reporting nothing.
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addresses {
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}

// Collector samples live host state. It holds no resources; the zero value is
// ready to use.
type Collector struct {
	gpu gpuProbe
}

// NewCollector returns a collector that will also report GPUs when the host
// has any it can query.
func NewCollector() *Collector {
	return &Collector{gpu: detectGPUProbe()}
}

// Telemetry samples the host's current CPU, memory and GPU state.
//
// The CPU figure is utilisation since the previous call, which means the first
// sample after startup is measured from process start and the ones after that
// each cover one heartbeat interval. Sampling this way costs nothing; asking
// for an instantaneous figure would mean blocking the heartbeat for a second
// to measure one.
func (c *Collector) Telemetry(ctx context.Context) *agentproto.AgentTelemetry {
	telemetry := &agentproto.AgentTelemetry{}

	if percentages, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(percentages) > 0 {
		telemetry.CpuPercent = percentages[0]
	}

	if virtualMemory, err := mem.VirtualMemoryWithContext(ctx); err == nil && virtualMemory != nil {
		telemetry.MemoryUsedBytes = virtualMemory.Used
		telemetry.MemoryTotalBytes = virtualMemory.Total
	}

	if c.gpu != nil {
		telemetry.Gpus = c.gpu.probe(ctx)
	}

	return telemetry
}
