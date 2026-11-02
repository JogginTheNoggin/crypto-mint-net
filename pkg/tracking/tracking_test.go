package tracking_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

var t0 = time.Unix(1_800_000_000, 0)

const sec = "p:00/01/02"

func newTracker(t *testing.T, cfg tracking.Config) *tracking.Tracker {
	t.Helper()
	tr, err := tracking.New(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestCounterWindow(t *testing.T) {
	var c tracking.Counter
	w := 80 * time.Second // 10 s slots
	c.Add(t0, w, 3)
	c.Add(t0.Add(30*time.Second), w, 2)
	if n := c.Sum(t0.Add(40*time.Second), w); n != 5 {
		t.Fatalf("sum %d, want 5", n)
	}
	// The first events slide out once their slot is a window old.
	if n := c.Sum(t0.Add(85*time.Second), w); n != 2 {
		t.Fatalf("sum after window %d, want 2", n)
	}
	if n := c.Sum(t0.Add(time.Hour), w); n != 0 {
		t.Fatalf("sum much later %d, want 0", n)
	}
	// A slot reused in a later round starts from zero.
	c.Add(t0.Add(80*time.Second), w, 1)
	if n := c.Sum(t0.Add(80*time.Second), w); n != 3 {
		t.Fatalf("sum after reuse %d, want 3", n)
	}
}

func TestAverage(t *testing.T) {
	var a tracking.Average
	a.Add(1)
	if a.Value != 1 {
		t.Fatal(a.Value)
	}
	for i := 0; i < 50; i++ {
		a.Add(3)
	}
	if a.Value < 2.99 || a.Value > 3 {
		t.Fatalf("average %v, want close to 3", a.Value)
	}
}

func TestTemplates(t *testing.T) {
	if !tracking.Standard.Has(tracking.Basic) || !tracking.Detailed.Has(tracking.Standard) {
		t.Fatal("templates do not nest")
	}
	tpl, err := tracking.NewTemplates(map[string]tracking.Custom{
		"lean":    {From: "mine", Off: []string{"rtt"}},
		"mine":    {On: []string{"request-outcomes"}},
		"logging": {From: "detailed", Off: []string{"traffic"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mine, _ := tpl.Get("mine")
	if !mine.Has(tracking.Basic|tracking.RequestOutcomes) || mine.Has(tracking.ReplyTiming) {
		t.Fatalf("mine = %v", mine)
	}
	lean, _ := tpl.Get("lean")
	if lean.Has(tracking.RTT) || !lean.Has(tracking.RequestOutcomes) {
		t.Fatalf("lean = %v", lean)
	}
	if _, err := tracking.NewTemplates(map[string]tracking.Custom{"basic": {}}); err == nil {
		t.Fatal("redefined a supplied template")
	}
	if _, err := tracking.NewTemplates(map[string]tracking.Custom{"a": {From: "b"}, "b": {From: "a"}}); err == nil {
		t.Fatal("accepted a cycle")
	}
	if _, err := tracking.NewTemplates(map[string]tracking.Custom{"a": {On: []string{"nope"}}}); err == nil {
		t.Fatal("accepted an unknown setting")
	}
	if _, err := tracking.New(tracking.Config{Default: "nope"}, ""); err == nil {
		t.Fatal("accepted an unknown default template")
	}
}

func TestKeys(t *testing.T) {
	tr := newTracker(t, tracking.Config{})
	for addr, want := range map[string]string{
		"10.0.0.1:8646":    "10.0.0.1",
		"10.0.0.1":         "10.0.0.1",
		"[2001:db8::1]:80": "2001:db8::1",
		"Node.Example:1":   "node.example",
		"127.0.0.1:5000":   "127.0.0.1:5000",
	} {
		if got := tr.Key(addr); got != want {
			t.Errorf("Key(%q) = %q, want %q", addr, got, want)
		}
	}
	one := newTracker(t, tracking.Config{LoopbackAsOne: true})
	if got := one.Key("127.0.0.1:5000"); got != "127.0.0.1" {
		t.Errorf("LoopbackAsOne key %q", got)
	}
}

func TestFlagsTimeoutsAndBlocks(t *testing.T) {
	tr := newTracker(t, tracking.Config{})
	a := "10.0.0.1:1"
	if st := tr.Status(a, t0); !st.Allowed() || !st.Priority {
		t.Fatalf("fresh address %+v", st)
	}
	// Severity 5 reaches the priority limit.
	if act := tr.Offend(a, tracking.InvalidItem, "bad entry", t0); act != tracking.None {
		t.Fatalf("first offence %v", act)
	}
	if st := tr.Status("10.0.0.1:2", t0); st.Priority || st.FlagTotal != 5 {
		t.Fatalf("after one offence %+v (another port, the same address)", st)
	}
	// 10 reaches the timeout limit.
	if act := tr.Offend(a, tracking.InvalidItem, "bad entry", t0.Add(time.Second)); act != tracking.TimeOut {
		t.Fatalf("second offence %v", act)
	}
	st := tr.Status(a, t0.Add(time.Minute))
	if !st.TimedOut || st.Allowed() {
		t.Fatalf("not timed out: %+v", st)
	}
	if st := tr.Status(a, t0.Add(11*time.Minute)); st.TimedOut {
		t.Fatalf("timeout did not end: %+v", st)
	}
	// Flags expire after a day and the peer may be a priority peer again.
	if st := tr.Status(a, t0.Add(25*time.Hour)); st.FlagTotal != 0 || !st.Priority {
		t.Fatalf("flags did not expire: %+v", st)
	}
	// Repeated timeouts turn into a block, even once the flags behind
	// each have expired.
	cfg := tracking.DefaultConfig()
	cfg.Offences[tracking.InvalidItem] = tracking.Rule{Severity: 5, Expiry: 30 * time.Minute}
	tr = newTracker(t, cfg)
	b := "10.9.0.1:1"
	for i := 0; i < 3; i++ {
		at := t0.Add(time.Duration(i) * time.Hour)
		tr.Offend(b, tracking.InvalidItem, "", at)
		if act := tr.Offend(b, tracking.InvalidItem, "", at); act != tracking.TimeOut {
			t.Fatalf("round %d: %v", i, act)
		}
	}
	at := t0.Add(3 * time.Hour)
	tr.Offend(b, tracking.InvalidItem, "", at)
	tr.Offend(b, tracking.InvalidItem, "", at)
	if st := tr.Status(b, at); !st.Blocked {
		t.Fatalf("not blocked after repeated timeouts: %+v", st)
	}
	tr.Lift(b)
	if st := tr.Status(b, at); !st.Allowed() {
		t.Fatalf("lift: %+v", st)
	}
}

func TestImmediateActionAndRanges(t *testing.T) {
	cfg := tracking.DefaultConfig()
	cfg.Offences[tracking.Contradiction] = tracking.Rule{Severity: 1, Expiry: time.Hour, Action: tracking.Block}
	tr := newTracker(t, cfg)
	if act := tr.Offend("10.0.0.1:1", tracking.Contradiction, "", t0); act != tracking.Block {
		t.Fatalf("immediate action %v", act)
	}
	rng := tr.BlockRange("10.5.1.1:1", time.Hour, t0)
	if rng != "v4:10.5" {
		t.Fatalf("range %q", rng)
	}
	if st := tr.Status("10.5.200.3:9", t0); !st.Blocked {
		t.Fatalf("range block missed: %+v", st)
	}
	if st := tr.Status("10.5.200.3:9", t0.Add(2*time.Hour)); st.Blocked {
		t.Fatalf("range block did not end: %+v", st)
	}
	tr.BlockFor("10.6.0.1", -1, t0)
	if st := tr.Status("10.6.0.1:1", t0.Add(24*365*time.Hour)); !st.Blocked {
		t.Fatal("block until lifted ended")
	}
}

func TestBudgetsAndRepeats(t *testing.T) {
	cfg := tracking.DefaultConfig()
	cfg.Budgets = map[string]int{tracking.KindTx: 3}
	cfg.RepeatLimit = 2
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	for i := 0; i < 3; i++ {
		if ok, _ := tr.Budget(a, tracking.KindTx, t0); !ok {
			t.Fatalf("message %d over budget", i)
		}
	}
	ok, act := tr.Budget(a, tracking.KindTx, t0)
	if ok || act != tracking.None {
		t.Fatalf("fourth: ok %v action %v", ok, act)
	}
	tr.Budget(a, tracking.KindTx, t0) // over again, same window: no second flag
	if st := tr.Status(a, t0); st.FlagTotal != 1 {
		t.Fatalf("flag total %v, want one over-budget flag", st.FlagTotal)
	}
	if ok, _ := tr.Budget(a, tracking.KindTx, t0.Add(2*time.Minute)); !ok {
		t.Fatal("budget did not recover")
	}
	if ok, _ := tr.Budget(a, tracking.KindLayer, t0); !ok {
		t.Fatal("a kind with no budget was limited")
	}
	tr.Repeat(a, t0)
	tr.Repeat(a, t0)
	if st := tr.Status(a, t0); st.FlagTotal != 1 {
		t.Fatal("repeats within the limit were flagged")
	}
	tr.Repeat(a, t0)
	if st := tr.Status(a, t0); st.FlagTotal != 3 {
		t.Fatalf("flag total %v after a repeat flood", st.FlagTotal)
	}
}

// RED-011: Spend draws a cost of many units from one budget.
func TestSpend(t *testing.T) {
	cfg := tracking.DefaultConfig()
	cfg.Budgets = map[string]int{tracking.KindRevert: 100}
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	if ok, _ := tr.Spend(a, tracking.KindRevert, 0, t0); !ok {
		t.Fatal("a free message was refused")
	}
	if ok, _ := tr.Spend(a, tracking.KindRevert, 60, t0); !ok {
		t.Fatal("60 of 100 refused")
	}
	if ok, _ := tr.Spend(a, tracking.KindRevert, 60, t0); ok {
		t.Fatal("120 of 100 allowed")
	}
	if st := tr.Status(a, t0); st.FlagTotal != 1 {
		t.Fatalf("flag total %v, want one over-budget flag", st.FlagTotal)
	}
	if ok, _ := tr.Spend("10.1.0.1:1", tracking.KindRevert, 100, t0); !ok {
		t.Fatal("another address shares the budget")
	}
	if ok, _ := tr.Spend(a, tracking.KindRevert, 60, t0.Add(2*time.Minute)); !ok {
		t.Fatal("budget did not recover")
	}
}

func TestPoints(t *testing.T) {
	tr := newTracker(t, tracking.Config{})
	good, bad := "10.0.0.1:1", "10.1.0.1:1"
	if p := tr.Points(good, sec, 0, t0); p != 0 {
		t.Fatalf("a new peer starts with %v points", p)
	}
	for i := 0; i < 10; i++ {
		at := t0.Add(time.Duration(i) * time.Minute)
		tr.Ping(good, sec, true, 20*time.Millisecond, at)
		tr.Ping(bad, sec, i%2 == 0, 400*time.Millisecond, at)
		tr.FirstDelivery(good, sec, "entry", at)
		tr.Request(good, sec, tracking.Answered, 50*time.Millisecond, 1000, at)
		tr.Request(bad, sec, tracking.TimedOut, 0, 0, at)
	}
	now := t0.Add(10 * time.Minute)
	pg, pb := tr.Points(good, sec, 0, now), tr.Points(bad, sec, 0, now)
	if pg <= pb {
		t.Fatalf("good peer %v points, bad %v", pg, pb)
	}
	// Only pings and first deliveries, the measures a backup has too.
	fair := tracking.Pings | tracking.RTT | tracking.FirstDeliveries
	if f := tr.Points(good, sec, fair, now); f >= pg {
		t.Fatalf("fair points %v should leave out requests (%v)", f, pg)
	}
	// Activity slides out of the window: a peer gone quiet loses points.
	if p := tr.Points(good, sec, 0, now.Add(3*time.Hour)); p >= pg {
		t.Fatalf("quiet peer kept %v points", p)
	}
	// Flags never subtract points.
	tr.Offend(good, tracking.InvalidItem, "", now)
	if p := tr.Points(good, sec, 0, now); p != pg {
		t.Fatalf("points %v changed by a flag (was %v)", p, pg)
	}
}

func TestStandardMeasures(t *testing.T) {
	cfg := tracking.Config{Default: tracking.StandardName}
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	tr.ChaseOutcome(a, sec, true, t0)
	base := tr.Points(a, sec, 0, t0)
	tr.Denial(a, sec, t0)
	if tr.Denials(a, sec, t0) != 1 {
		t.Fatal("denial not counted")
	}
	if p := tr.Points(a, sec, 0, t0); p >= base {
		t.Fatalf("a denial did not lower points: %v, was %v", p, base)
	}
	tr.Cost(a, sec, 10<<20, 0, true, t0)
	if p := tr.Points(a, sec, 0, t0); p >= base-2 {
		t.Fatalf("cost did not lower points: %v", p)
	}
	tr.Stated(a, sec, weight.FromUint64(7), t0)
	tr.Stated(a, sec, weight.FromUint64(5), t0)
	rep, ok := tr.Report(a, t0)
	if !ok || rep.Policies[sec].Highest != "7" {
		t.Fatalf("report %+v", rep)
	}
	// The basic template records none of these.
	b := newTracker(t, tracking.Config{})
	b.Denial(a, sec, t0)
	b.Cost(a, sec, 10<<20, 0, true, t0)
	if p := b.Points(a, sec, 0, t0); p != 0 {
		t.Fatalf("basic template recorded denials or cost: %v", p)
	}
}

func TestTemplateChoice(t *testing.T) {
	cfg := tracking.Config{
		Groups:     map[string]string{"priority": tracking.StandardName, "inbound": tracking.DetailedName},
		Peers:      map[string]string{"10.9.9.9": tracking.BasicName},
		EscalateAt: 5,
	}
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	if s := tr.Settings(a, t0); s != tracking.Basic {
		t.Fatalf("default %v", s)
	}
	tr.SetGroups(a, []string{"priority"}, t0)
	if s := tr.Settings(a, t0); s != tracking.Standard {
		t.Fatalf("group %v", s)
	}
	tr.SetGroups(a, []string{"priority", "inbound"}, t0)
	if s := tr.Settings(a, t0); s != tracking.Detailed {
		t.Fatalf("union of groups %v", s)
	}
	if err := tr.SetTemplate(a, tracking.BasicName, t0); err != nil {
		t.Fatal(err)
	}
	if s := tr.Settings(a, t0); s != tracking.Basic {
		t.Fatalf("own template %v", s)
	}
	if err := tr.SetTemplate(a, "nope", t0); err == nil {
		t.Fatal("unknown template accepted")
	}
	// The policy's own choice for an address.
	tr.SetGroups("10.9.9.9:1", []string{"inbound"}, t0)
	if s := tr.Settings("10.9.9.9:1", t0); s != tracking.Basic {
		t.Fatalf("policy choice %v", s)
	}
	// A rising flag total moves a basic peer onto the standard template.
	tr.Offend(a, tracking.InvalidItem, "", t0)
	if s := tr.Settings(a, t0); !s.Has(tracking.Standard) {
		t.Fatalf("not escalated: %v", s)
	}
}

func TestDetailedLog(t *testing.T) {
	cfg := tracking.Config{Default: tracking.DetailedName}
	cfg.Retain.LogSize = 3
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	for i := 0; i < 5; i++ {
		tr.Ping(a, sec, true, time.Millisecond, t0.Add(time.Duration(i)*time.Second))
	}
	tr.Message(a, "entry", 100, t0)
	rep, _ := tr.Report(a, t0.Add(5*time.Second))
	if len(rep.Log) != 3 || !rep.Log[2].At.Equal(t0.Add(4*time.Second)) {
		t.Fatalf("log %+v", rep.Log)
	}
	rep, _ = tr.Report(a, t0.Add(2*time.Hour))
	tr.Ping(a, sec, true, time.Millisecond, t0.Add(2*time.Hour))
	rep, _ = tr.Report(a, t0.Add(2*time.Hour))
	if len(rep.Log) != 1 {
		t.Fatalf("old log lines kept: %+v", rep.Log)
	}
}

func TestHeldCategoriesAndMissedPings(t *testing.T) {
	tr := newTracker(t, tracking.Config{})
	a := "10.0.0.1:1"
	tr.Ping(a, sec, true, time.Millisecond, t0)
	tr.TimeOutFor(a, 10*time.Minute, t0)
	tr.Hold(a, map[string]string{sec: "priority"}, t0)
	held := tr.Held(a, 2*time.Minute, t0.Add(10*time.Minute))
	if !reflect.DeepEqual(held, map[string]string{sec: "priority"}) {
		t.Fatalf("held %v", held)
	}
	if tr.Held(a, 2*time.Minute, t0.Add(10*time.Minute)) != nil {
		t.Fatal("held categories returned twice")
	}
	rep, _ := tr.Report(a, t0.Add(10*time.Minute))
	if rep.Policies[sec].Missed != 5 {
		t.Fatalf("missed %d pings during the timeout, want 5", rep.Policies[sec].Missed)
	}
}

func TestClearAndForget(t *testing.T) {
	tr := newTracker(t, tracking.Config{})
	a := "10.0.0.1:1"
	tr.Ping(a, sec, true, time.Millisecond, t0)
	tr.Offend(a, tracking.InvalidItem, "", t0)
	tr.Offend(a, tracking.InvalidItem, "", t0)
	if err := tr.Clear(a, "flags"); err != nil {
		t.Fatal(err)
	}
	st := tr.Status(a, t0)
	if st.FlagTotal != 0 || !st.TimedOut {
		t.Fatalf("clearing flags: %+v (the timeout stays)", st)
	}
	tr.Forget(a, sec)
	if rep, _ := tr.Report(a, t0); len(rep.Policies) != 0 {
		t.Fatal("forget kept measures")
	}
	if err := tr.Clear(a, "all"); err != nil {
		t.Fatal(err)
	}
	if st := tr.Status(a, t0); !st.Allowed() {
		t.Fatal("clearing all kept the timeout")
	}
	if err := tr.Clear(a, "nope"); err == nil {
		t.Fatal("unknown clear accepted")
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), tracking.FileName)
	tr, err := tracking.New(tracking.Config{}, path)
	if err != nil {
		t.Fatal(err)
	}
	a := "10.0.0.1:1"
	tr.Ping(a, sec, true, 10*time.Millisecond, t0)
	tr.FirstDelivery(a, sec, "entry", t0)
	tr.Offend(a, tracking.InvalidItem, "x", t0)
	tr.BlockRange("10.7.0.1:1", time.Hour, t0)
	if err := tr.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := tracking.New(tracking.Config{}, path)
	if err != nil {
		t.Fatal(err)
	}
	if p, q := tr.Points(a, sec, 0, t0), back.Points(a, sec, 0, t0); p != q || p == 0 {
		t.Fatalf("points %v after reload, %v before", q, p)
	}
	if st := back.Status(a, t0); st.FlagTotal != 5 {
		t.Fatalf("flags lost: %+v", st)
	}
	if st := back.Status("10.7.3.3:1", t0); !st.Blocked {
		t.Fatal("range block lost")
	}
}

func TestTrim(t *testing.T) {
	cfg := tracking.Config{}
	cfg.Retain.MaxAddresses = 32
	tr := newTracker(t, cfg)
	tr.Offend("10.200.0.1:1", tracking.InvalidItem, "", t0)
	for i := 0; i < 100; i++ {
		tr.Heard("10.0.0."+itoa(i)+":1", sec, t0.Add(time.Duration(i)*time.Second))
	}
	if n := len(tr.Keys()); n > 32 {
		t.Fatalf("%d records kept, limit 32", n)
	}
	if st := tr.Status("10.200.0.1:1", t0); st.FlagTotal != 5 {
		t.Fatal("trimming dropped a flagged address")
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// Only what is taken counts against a budget (D121): a sender at twice
// the budget's rate still gets the budget's rate through, minute after
// minute, instead of being starved by its own dropped messages.
func TestSteadySenderIsNotStarved(t *testing.T) {
	cfg := tracking.DefaultConfig()
	cfg.Budgets = map[string]int{tracking.KindLayer: 30}
	tr := newTracker(t, cfg)
	a := "10.0.0.1:1"
	took := 0
	for i := 0; i < 360; i++ { // two a second for three minutes
		if ok, _ := tr.Budget(a, tracking.KindLayer, t0.Add(time.Duration(i)*500*time.Millisecond)); ok {
			took++
		}
	}
	if took < 75 {
		t.Fatalf("took %d of 360 in three minutes at a budget of 30 a minute", took)
	}
}
