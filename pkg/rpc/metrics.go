package rpc

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// Metrics: GET /v1/metrics serves what /v1/status and /v1/peers report as
// Prometheus text exposition format 0.0.4, so a node can be scraped and
// graphed without a sidecar. It needs the token like every endpoint (a
// scraper sends it as a bearer token). It reports only what the node
// already reports; it adds no counters of its own (GREEN-002).

const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// metric is one family: its name, help, type and samples.
type metric struct {
	name, help, kind string
	samples          []sample
}

type sample struct {
	labels string // already formatted: {a="b",c="d"}, or empty
	value  float64
}

func (m *metric) add(labels string, v float64) { m.samples = append(m.samples, sample{labels, v}) }

func writeMetrics(w io.Writer, ms []*metric) {
	for _, m := range ms {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", m.name, m.help, m.name, m.kind)
		for _, s := range m.samples {
			fmt.Fprintf(w, "%s%s %v\n", m.name, s.labels, s.value)
		}
	}
}

// labelValue escapes a label value as the text format requires.
func labelValue(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	g := func(name, help string) *metric { return &metric{name: "poo_" + name, help: help, kind: "gauge"} }
	height := g("pool_height", "Height of the selected ledger of a followed pool.")
	weight := g("pool_weight", "Weight of the selected ledger (a float: exact weights are in /v1/status).")
	entries := g("pool_entries", "Entries the pool's ledger keeps.")
	branches := g("pool_branches", "Branches the pool's ledger keeps.")
	pending := g("pool_pending", "Pending transactions of the pool.")
	waiting := g("pool_waiting", "Transactions waiting on an unknown input.")
	stored := g("pool_stored_records", "Records in the pool's store.")
	stores := map[string]int{}
	for _, s := range h.n.Stores() {
		stores[s.Key.Series.String()+"/"+fmt.Sprint(s.Key.Index)] = s.Records
	}
	for _, k := range h.n.Engine().FollowedPools() {
		st, err := h.n.Engine().Status(k)
		if err != nil {
			writeError(w, classify(err))
			return
		}
		l := fmt.Sprintf(`{series="%s",pool="%d"}`, k.Series, k.Index)
		height.add(l, float64(st.Height))
		f, _ := strconv.ParseFloat(st.Weight.String(), 64)
		weight.add(l, f)
		entries.add(l, float64(st.Entries))
		branches.add(l, float64(len(st.Branches)))
		pending.add(l, float64(st.Pending))
		waiting.add(l, float64(st.Waiting))
		stored.add(l, float64(stores[k.Series.String()+"/"+fmt.Sprint(k.Index)]))
	}
	ms := []*metric{height, weight, entries, branches, pending, waiting, stored}
	if h.host != nil {
		ps := h.host.Peers()
		conn := g("peers_connected", "Connected peers, by direction.")
		var in, out float64
		cats := map[string]float64{}
		sent := &metric{name: "poo_peer_sent_total", help: "Messages sent to connected peers.", kind: "counter"}
		recv := &metric{name: "poo_peer_received_total", help: "Messages received from connected peers.", kind: "counter"}
		var s, rc float64
		for _, p := range ps {
			if p.Outbound {
				out++
			} else {
				in++
			}
			for _, st := range p.Categories {
				cats[st.Category]++
			}
			s += float64(p.Sent)
			rc += float64(p.Received)
		}
		conn.add(`{direction="inbound"}`, in)
		conn.add(`{direction="outbound"}`, out)
		sent.add("", s)
		recv.add("", rc)
		cat := g("peer_categories", "Connected peers per category, counted once per mint policy they were verified for.")
		names := make([]string, 0, len(cats))
		for c := range cats {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			cat.add(fmt.Sprintf(`{category="%s"}`, labelValue(c)), cats[c])
		}
		book := g("peer_book_records", "Nodes in the peer book.")
		book.add("", float64(len(h.host.Book().Records())))
		disk := g("chases_waiting_disk", "Chases of a heavier stated ledger waiting for disk space for a large entry.")
		nd := 0
		for _, gr := range h.host.ChaseQueue().Groups("", time.Now()) {
			if gr.Waiting == p2p.WaitDisk {
				nd++
			}
		}
		disk.add("", float64(nd))
		ms = append(ms, conn, cat, sent, recv, book, disk)
	}
	ms = append(ms, h.grindMetrics()...)
	w.Header().Set("Content-Type", metricsContentType)
	writeMetrics(w, ms)
}
