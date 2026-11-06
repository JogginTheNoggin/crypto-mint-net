// Package brand holds the project's name as the software shows and writes
// it, in one place, so a later rename is one change here (D192). Nothing
// here is consensus: no hash, id or signature covers these strings.
//
// Some strings carry the name but must never follow a rename, because a
// change would break what earlier releases made or would split nodes:
//   - the wallet's key derivation label "PoO-wallet-..." (pkg/wallet):
//     a new label gives every wallet new keys;
//   - the RPC hello proof label "poo-rpc-hello" (pkg/rpc): a client and
//     node of different releases would no longer prove each other;
//   - every byte form and wire layout (frozen, D76, D186).
//
// Those stay where they are, written out, with a comment pointing here.
//
// The name was "poo" until Source Round 40, then "poon" (proof of
// ownership network, or node; D204). Help text, messages and docs write the
// name out, so a later rename changes Name and EnvPrefix here and then
// replaces the word as a whole word (not inside "pool" or the labels above)
// in pkg/, cmd/, scripts/ and the living docs, and moves cmd/NAME and
// docs/modules/NAME.md. Historical records (decisions, findings, proposals,
// reviews) keep the name they were written with.
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
