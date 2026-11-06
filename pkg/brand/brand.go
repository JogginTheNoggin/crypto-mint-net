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
package brand

const (
	// Name is the command, the binary and the project's short name.
	Name = "poo"
	// GUI is the GUI's binary.
	GUI = Name + "-gui"
	// DataDir is the data folder's name under the home directory.
	DataDir = "." + Name
	// Agent is what a node puts in its hello's agent field.
	Agent = Name
	// MetricPrefix starts every metric name.
	MetricPrefix = Name + "_"
	// KindPrefix starts the kind of every file poo writes for sharing
	// (node policies, peers, ledgers).
	KindPrefix = Name + "-"
	// TempPrefix starts the names of temporary files.
	TempPrefix = Name + "-"
)
