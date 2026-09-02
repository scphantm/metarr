package busstats

import (
	"context"
	"sort"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// samplerWithSlugs builds a sampler against a fresh miniredis whose registered
// agent set is fixed to slugs.
func samplerWithSlugs(t *testing.T, slugs ...string) (*Sampler, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client, nil, WithHistory(4), WithSlugSource(func(context.Context) []string {
		return slugs
	})), mr
}

// DeriveTopology is a pure function of the unified topic table and the
// registered agent slug set. This walks it for zero, one, and several agents.
func TestDeriveTopology(t *testing.T) {
	const server = "metarr-server"

	cases := []struct {
		name  string
		slugs []string
		// A row mapped to nil must be present with no expected identity (a
		// reserved stream).
		wantStreamIdentities  map[string][]string
		wantChannelIdentities map[string][]string
	}{
		{
			name:  "zero agents: static rows only",
			slugs: nil,
			wantStreamIdentities: map[string][]string{
				eventbus.SystemConfigUpdateStream: {server},
				eventbus.AgentScanResultStream:    {server},
				eventbus.AgentNodeResultStream:    nil,
			},
			wantChannelIdentities: map[string][]string{
				eventbus.HeartbeatRequestChannel: {server},
				eventbus.LogChannel:              {server},
			},
		},
		{
			name:  "one agent: its command stream and two channels appear",
			slugs: []string{"nas-01"},
			wantStreamIdentities: map[string][]string{
				eventbus.AgentCommandStream("nas-01"): {"metarr-agent-nas-01"},
			},
			wantChannelIdentities: map[string][]string{
				eventbus.AgentConfigChangedChannel("nas-01"): {"metarr-agent-nas-01"},
				eventbus.AgentRequestChannel("nas-01"):       {"metarr-agent-nas-01"},
			},
		},
		{
			name:  "several agents: one command stream and two channels each",
			slugs: []string{"nas-01", "tower", "media-2"},
			wantStreamIdentities: map[string][]string{
				eventbus.AgentCommandStream("nas-01"):  {"metarr-agent-nas-01"},
				eventbus.AgentCommandStream("tower"):   {"metarr-agent-tower"},
				eventbus.AgentCommandStream("media-2"): {"metarr-agent-media-2"},
			},
			wantChannelIdentities: map[string][]string{
				eventbus.AgentConfigChangedChannel("tower"): {"metarr-agent-tower"},
				eventbus.AgentRequestChannel("media-2"):     {"metarr-agent-media-2"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			top := DeriveTopology(eventbus.Topics(), tc.slugs)

			for name, want := range tc.wantStreamIdentities {
				got, ok := top.Streams[name]
				if !ok {
					t.Fatalf("stream %q missing from topology", name)
				}
				assertIdentities(t, "stream "+name, got.Identities, want)
			}
			for name, want := range tc.wantChannelIdentities {
				got, ok := top.Channels[name]
				if !ok {
					t.Fatalf("channel %q missing from topology", name)
				}
				assertIdentities(t, "channel "+name, got.Identities, want)
			}

			// A pattern row must never leak into the derived streams.
			if _, ok := top.Streams[eventbus.AgentCommandStreamPattern]; ok {
				t.Errorf("pattern row %q leaked into the topology", eventbus.AgentCommandStreamPattern)
			}
			// One command stream and two channels per agent, on top of the
			// three static streams and two static channels.
			if want := 3 + len(tc.slugs); len(top.Streams) != want {
				t.Errorf("stream count = %d, want %d", len(top.Streams), want)
			}
			if want := 2 + 2*len(tc.slugs); len(top.Channels) != want {
				t.Errorf("channel count = %d, want %d", len(top.Channels), want)
			}
		})
	}
}

// DeriveTopology takes the one unified topic table and splits it by Kind: a
// KindStream row lands in Streams, a KindNotify or KindRequestReply row in
// Channels. It no longer reads a streams-only list plus KnownPubSubChannels
// plus AgentPubSubChannels — feeding it eventbus.Topics() alone must yield
// both the stream rows and the fixed channel rows.
func TestDeriveTopologyConsumesTheUnifiedTable(t *testing.T) {
	top := DeriveTopology(eventbus.Topics(), []string{"nas-01"})

	// KindStream rows -> Streams.
	for _, name := range []string{
		eventbus.SystemConfigUpdateStream,
		eventbus.AgentScanResultStream,
		eventbus.AgentNodeResultStream,
		eventbus.AgentCommandStream("nas-01"),
	} {
		if _, ok := top.Streams[name]; !ok {
			t.Errorf("stream %q missing from Streams", name)
		}
		if _, wrong := top.Channels[name]; wrong {
			t.Errorf("stream %q wrongly landed in Channels", name)
		}
	}

	// KindRequestReply / KindNotify rows -> Channels, including the fixed
	// channels that used to come from a separate KnownPubSubChannels call.
	for _, name := range []string{
		eventbus.HeartbeatRequestChannel, // request_reply
		eventbus.LogChannel,              // notify
		eventbus.AgentConfigChangedChannel("nas-01"),
		eventbus.AgentRequestChannel("nas-01"),
	} {
		if _, ok := top.Channels[name]; !ok {
			t.Errorf("channel %q missing from Channels", name)
		}
		if _, wrong := top.Streams[name]; wrong {
			t.Errorf("channel %q wrongly landed in Streams", name)
		}
	}

	// The pattern row is skipped, not placed anywhere.
	if _, ok := top.Streams[eventbus.AgentCommandStreamPattern]; ok {
		t.Errorf("pattern row %q leaked into Streams", eventbus.AgentCommandStreamPattern)
	}
}

func assertIdentities(t *testing.T, where string, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Fatalf("%s: identities = %v, want %v", where, got, want)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("%s: identities = %v, want %v", where, got, want)
		}
	}
}

// nameMissing attributes a shortfall: an agent identity whose presence key is
// absent, an agent identity where the count itself is short, and the server
// identity only when the count is short. A present agent on a row that is not
// count-short yields nothing.
func TestNameMissing(t *testing.T) {
	present := map[string]bool{"online": true}

	cases := []struct {
		name       string
		expected   []string
		countShort bool
		want       []string
	}{
		{"offline agent named regardless of count", []string{"metarr-agent-offline"}, false, []string{"metarr-agent-offline"}},
		{"present agent, count not short: nothing", []string{"metarr-agent-online"}, false, nil},
		{"present agent, count short: named", []string{"metarr-agent-online"}, true, []string{"metarr-agent-online"}},
		{"server, count not short: nothing", []string{"metarr-server"}, false, nil},
		{"server, count short: named", []string{"metarr-server"}, true, []string{"metarr-server"}},
		{
			"mixed, count not short: only the offline agent",
			[]string{"metarr-agent-online", "metarr-agent-offline"},
			false,
			[]string{"metarr-agent-offline"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertIdentities(t, tc.name, nameMissing(tc.expected, present, tc.countShort), tc.want)
		})
	}
}

// A server-consumed static stream whose consumer group has not been read yet
// — a fresh start, before the Router's first XREADGROUP — is not flagged:
// durable streams are created lazily and that is cold-start, not a fault.
func TestApplyStreamTopologyDoesNotFlagColdStartServerStream(t *testing.T) {
	top := DeriveTopology(eventbus.Topics(), nil)
	// The scan-result stream row exists but has no groups yet.
	streams := []*StreamStat{{Stream: eventbus.AgentScanResultStream, Groups: []*GroupStat{}}}

	got := applyStreamTopology(streams, top, map[string]bool{})

	row := findStream(got, eventbus.AgentScanResultStream)
	if row == nil {
		t.Fatalf("%s row went missing", eventbus.AgentScanResultStream)
	}
	if !containsString(row.ExpectedIdentities, "metarr-server") {
		t.Errorf("expected identities = %v, want metarr-server", row.ExpectedIdentities)
	}
	if row.Flagged {
		t.Errorf("%s should not be flagged before its group is created", eventbus.AgentScanResultStream)
	}
}

// The same stream is flagged once its group exists but nothing is consuming
// it — a real "the server stopped reading this" fault.
func TestApplyStreamTopologyFlagsUnconsumedServerGroup(t *testing.T) {
	top := DeriveTopology(eventbus.Topics(), nil)
	streams := []*StreamStat{{
		Stream: eventbus.AgentScanResultStream,
		Groups: []*GroupStat{{Name: eventbus.AgentScanResultGroup, Consumers: 0}},
	}}

	got := applyStreamTopology(streams, top, map[string]bool{})

	row := findStream(got, eventbus.AgentScanResultStream)
	if row == nil || !row.Flagged {
		t.Fatalf("%s should be flagged: its group exists with no consumer", eventbus.AgentScanResultStream)
	}
	if !containsString(row.MissingIdentities, "metarr-server") {
		t.Errorf("missing identities = %v, want metarr-server", row.MissingIdentities)
	}
}

// A registered agent that is offline — no presence key, no command-stream key,
// no channel subscriber — still produces rows, each flagged with the agent
// named as the missing identity.
func TestApplyTopologyFlagsDisconnectedAgent(t *testing.T) {
	sampler, _ := samplerWithSlugs(t, "ghost")

	sampler.pass(context.Background())
	snap := sampler.Get()

	command := eventbus.AgentCommandStream("ghost")
	stream := findStream(snap.GetStreams(), command)
	if stream == nil {
		t.Fatalf("offline agent's command stream %q is not a row", command)
	}
	if !stream.GetFlagged() {
		t.Errorf("%s should be flagged: the agent is not consuming it", command)
	}
	if !containsString(stream.GetMissingIdentities(), "metarr-agent-ghost") {
		t.Errorf("%s missing identities = %v, want metarr-agent-ghost", command, stream.GetMissingIdentities())
	}
	if !containsString(stream.GetExpectedIdentities(), "metarr-agent-ghost") {
		t.Errorf("%s expected identities = %v, want metarr-agent-ghost", command, stream.GetExpectedIdentities())
	}

	for _, name := range eventbus.AgentPubSubChannels("ghost") {
		ch := findChannel(snap.GetChannels(), name)
		if ch == nil {
			t.Fatalf("offline agent's channel %q is not a row", name)
		}
		if !ch.GetKnown() {
			t.Errorf("%s should be known: it is a declared per-agent channel", name)
		}
		if !ch.GetFlagged() {
			t.Errorf("%s should be flagged: the agent is not subscribed", name)
		}
		if !containsString(ch.GetMissingIdentities(), "metarr-agent-ghost") {
			t.Errorf("%s missing identities = %v, want metarr-agent-ghost", name, ch.GetMissingIdentities())
		}
	}
}

// A row whose live count meets its expected identity count is not flagged: a
// live presence key plus a real consumer on the expected group.
func TestApplyTopologyDoesNotFlagAttachedAgent(t *testing.T) {
	slug := "nas-01"
	sampler, _ := samplerWithSlugs(t, slug)
	client := sampler.client
	ctx := context.Background()

	if err := client.Set(ctx, agentproto.PresenceKey(slug), "{}", 0).Err(); err != nil {
		t.Fatalf("seed presence: %v", err)
	}
	command := eventbus.AgentCommandStream(slug)
	group := eventbus.AgentCommandGroup(slug)
	if err := client.XGroupCreateMkStream(ctx, command, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: command, Values: map[string]any{"k": "v"}}).Err(); err != nil {
		t.Fatalf("seed entry: %v", err)
	}
	if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: group, Consumer: eventbus.AgentSource(slug), Streams: []string{command, ">"}, Count: 1,
	}).Err(); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	sampler.pass(ctx)

	stream := findStream(sampler.Get().GetStreams(), command)
	if stream == nil {
		t.Fatalf("command stream %q is not a row", command)
	}
	if stream.GetFlagged() {
		t.Errorf("%s should not be flagged: a consumer is attached (expected %v, missing %v)",
			command, stream.GetExpectedIdentities(), stream.GetMissingIdentities())
	}
}

func findChannel(channels []*ChannelStat, name string) *ChannelStat {
	for _, c := range channels {
		if c.GetChannel() == name {
			return c
		}
	}
	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
