// Package brand holds the project's name as the software shows and writes
// it, in one place, so a later rename is one change here (D192). Nothing
// here is consensus: no hash, id or signature covers these strings.
//
// The tool was renamed poon (proof of ownership network, or node) in
// Source Round 40 (D204), and every use of the old name went with it,
// labels included (D205); the protocol's labels became "PoON-" (D206,
// jerry's approval), since nothing was released.
//
// Some strings must never follow a later rename, because a change would
// break what earlier releases made or would split nodes:
//   - the RPC hello proof label "poon-rpc-hello" (pkg/rpc): a client and
//     node of different releases would no longer prove each other;
//   - the "PoON-" labels (wallet keys, hashes, snapshots): a new hash label
//     changes every id, which is a consensus change, and a new wallet label
//     gives every wallet new keys;
//   - every byte form and wire layout (frozen, D76, D186).
//
// Those stay where they are, written out, with a comment pointing here.
// Help text, messages and docs write the name out, so a later rename
// changes Name and EnvPrefix here and then replaces the word as a whole
// word (not inside "pool") in the code and docs, and moves cmd/NAME and
// docs/modules/NAME.md.
package brand

const (
	// Name is the command, the binary and the project's short name.
	Name = "poon"
	// GUI is the GUI's binary.
	GUI = Name + "-gui"
	// DataDir is the data folder's name under the home directory.
	DataDir = "." + Name
	// Agent is what a node puts in its hello's agent field.
	Agent = Name
	// MetricPrefix starts every metric name.
	MetricPrefix = Name + "_"
	// KindPrefix starts the kind of every file poon writes for sharing
	// (node policies, peers, ledgers).
	KindPrefix = Name + "-"
	// TempPrefix starts the names of temporary files.
	TempPrefix = Name + "-"
	// EnvPrefix starts every environment variable the tool reads
	// (POON_HOME, POON_PASSPHRASE, POON_SEED, ...).
	EnvPrefix = "POON_"
)
