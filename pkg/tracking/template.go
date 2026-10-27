package tracking

import (
	"fmt"
	"sort"
	"strings"
)

// Setting is one thing a node can choose to record about its peers. A
// template is a set of settings.
type Setting uint32

// Node-wide settings, kept per address.
const (
	// Offences records invalid items and messages as flags.
	Offences Setting = 1 << iota
	// Budgets counts messages by kind against their budget.
	Budgets
	// Repeats counts requests and items already on the seen list.
	Repeats
	// Chases counts the chases a sender started against its budget.
	Chases
	// Contradictions records a peer contradicting what it said before.
	Contradictions
	// Connections counts connections opened, closed and refused.
	Connections
	// Log keeps every event with its time.
	Log
	// Traffic counts messages and bytes by message type.
	Traffic

	// LastHeard keeps when the peer was last heard from, per mint policy.
	LastHeard
	// Pings counts pings sent, answered and missed.
	Pings
	// RTT keeps the running average ping round trip.
	RTT
	// FirstDeliveries counts new valid items this peer sent before any
	// other.
	FirstDeliveries
	// Requests counts requests sent and how many were answered or timed
	// out.
	Requests
	// RequestOutcomes counts refused and not held answers as well.
	RequestOutcomes
	// Items counts valid items received, by kind.
	Items
	// ReplyTiming keeps running averages of reply time and throughput.
	ReplyTiming
	// StatedWeights keeps the highest stated weight per pool.
	StatedWeights
	// ChaseOutcomes counts stated weights that started a chase and how
	// many were confirmed or led nowhere.
	ChaseOutcomes
	// Denials counts a priority peer saying it saw nothing heavier when a
	// heavier ledger existed.
	Denials
	// Cost counts bytes served to and received from the peer, and large
	// requests it made.
	Cost

	settingEnd
)

var settingNames = map[Setting]string{
	Offences: "offences", Budgets: "budgets", Repeats: "repeats", Chases: "chases",
	Contradictions: "contradictions", Connections: "connections", Log: "log", Traffic: "traffic",
	LastHeard: "last-heard", Pings: "pings", RTT: "rtt", FirstDeliveries: "first-deliveries",
	Requests: "requests", RequestOutcomes: "request-outcomes", Items: "items",
	ReplyTiming: "reply-timing", StatedWeights: "stated-weights", ChaseOutcomes: "chase-outcomes",
	Denials: "denials", Cost: "cost",
}

// ParseSetting reads a setting's name.
func ParseSetting(name string) (Setting, error) {
	for s, n := range settingNames {
		if n == name {
			return s, nil
		}
	}
	return 0, fmt.Errorf("tracking: unknown setting %q", name)
}

// Has reports whether every setting in o is in s.
func (s Setting) Has(o Setting) bool { return s&o == o }

// Names lists the settings in s, in a fixed order.
func (s Setting) Names() []string {
	var out []string
	for b := Setting(1); b < settingEnd; b <<= 1 {
		if s&b != 0 {
			out = append(out, settingNames[b])
		}
	}
	return out
}

func (s Setting) String() string { return strings.Join(s.Names(), ",") }

// The supplied templates. Each includes everything in the one before.
const (
	Basic    = Offences | Budgets | Repeats | Chases | LastHeard | Pings | RTT | FirstDeliveries | Requests | ChaseOutcomes
	Standard = Basic | Contradictions | Connections | RequestOutcomes | Items | ReplyTiming | StatedWeights | Denials | Cost
	Detailed = Standard | Log | Traffic
)

// Names of the supplied templates, which cannot be replaced or deleted.
const (
	BasicName    = "basic"
	StandardName = "standard"
	DetailedName = "detailed"
)

// Custom describes a template of the user's own: a copy of another
// template with settings turned on or off.
type Custom struct {
	// From names the template it starts from (empty: basic).
	From string   `json:"from,omitempty"`
	On   []string `json:"on,omitempty"`
	Off  []string `json:"off,omitempty"`
}

// Templates resolves template names.
type Templates struct {
	byName map[string]Setting
}

// NewTemplates builds the supplied templates and the custom ones, in an
// order where a custom template may start from any other.
func NewTemplates(custom map[string]Custom) (Templates, error) {
	t := Templates{byName: map[string]Setting{BasicName: Basic, StandardName: Standard, DetailedName: Detailed}}
	names := make([]string, 0, len(custom))
	for n := range custom {
		if _, ok := t.byName[n]; ok {
			return t, fmt.Errorf("tracking: template %q is supplied and cannot be redefined", n)
		}
		names = append(names, n)
	}
	sort.Strings(names)
	// Resolve in rounds, so a template may start from another custom one
	// whatever their names; a cycle never resolves.
	for len(names) > 0 {
		var left []string
		for _, n := range names {
			c := custom[n]
			from := c.From
			if from == "" {
				from = BasicName
			}
			base, ok := t.byName[from]
			if !ok {
				left = append(left, n)
				continue
			}
			for _, on := range c.On {
				s, err := ParseSetting(on)
				if err != nil {
					return t, fmt.Errorf("template %q: %w", n, err)
				}
				base |= s
			}
			for _, off := range c.Off {
				s, err := ParseSetting(off)
				if err != nil {
					return t, fmt.Errorf("template %q: %w", n, err)
				}
				base &^= s
			}
			t.byName[n] = base
		}
		if len(left) == len(names) {
			return t, fmt.Errorf("tracking: templates %v start from an unknown template or each other", left)
		}
		names = left
	}
	return t, nil
}

// Get returns a template's settings.
func (t Templates) Get(name string) (Setting, bool) {
	s, ok := t.byName[name]
	return s, ok
}

// Names lists the known templates.
func (t Templates) Names() []string {
	out := make([]string, 0, len(t.byName))
	for n := range t.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
