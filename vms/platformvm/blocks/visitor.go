

package blocks

type Visitor interface {
	BanffAbortBlock(*BanffAbortBlock) error
	BanffCommitBlock(*BanffCommitBlock) error
	BanffProposalBlock(*BanffProposalBlock) error
	BanffStandardBlock(*BanffStandardBlock) error

	ApricotAbortBlock(*ApricotAbortBlock) error
	ApricotCommitBlock(*ApricotCommitBlock) error
	ApricotProposalBlock(*ApricotProposalBlock) error
	ApricotStandardBlock(*ApricotStandardBlock) error
	ApricotAtomicBlock(*ApricotAtomicBlock) error
}
