package wire

// Bounty market messages (D196). They are sent only to a peer whose hello
// lists FeatureBounty; a node that does not take part ignores them, as it
// ignores any type it does not know. Their bodies are package bounty's
// forms. None of them is ever part of the ledger. Numbers are never
// reused.
const (
	TypeBountyOffer   Type = 70
	TypeBountyListReq Type = 71
	TypeBountyList    Type = 72
	TypeBountyTake    Type = 73
	TypeBountyGrant   Type = 74
	TypeBountyClaim   Type = 75
	TypeBountyAck     Type = 76
)

func init() {
	for t, n := range map[Type]string{
		TypeBountyOffer: "bounty-offer", TypeBountyListReq: "bounty-list-request", TypeBountyList: "bounty-list",
		TypeBountyTake: "bounty-take", TypeBountyGrant: "bounty-grant", TypeBountyClaim: "bounty-claim",
		TypeBountyAck: "bounty-ack",
	} {
		typeNames[t] = n
	}
}

// IsBounty reports whether t is a bounty market message.
func (t Type) IsBounty() bool { return t >= TypeBountyOffer && t <= TypeBountyAck }
