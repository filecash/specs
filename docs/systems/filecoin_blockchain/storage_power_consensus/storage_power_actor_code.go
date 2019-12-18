package storage_power_consensus

import (
	"math"

	libp2p "github.com/filecoin-project/specs/libraries/libp2p"
	block "github.com/filecoin-project/specs/systems/filecoin_blockchain/struct/block"
	sector "github.com/filecoin-project/specs/systems/filecoin_mining/sector"
	actor "github.com/filecoin-project/specs/systems/filecoin_vm/actor"
	addr "github.com/filecoin-project/specs/systems/filecoin_vm/actor/address"
	ai "github.com/filecoin-project/specs/systems/filecoin_vm/actor_interfaces"
	util "github.com/filecoin-project/specs/util"
)

////////////////////////////////////////////////////////////////////////////////
// Actor methods
////////////////////////////////////////////////////////////////////////////////

func (a *StoragePowerActorCode_I) AddBalance(rt Runtime, minerAddr addr.Address) {
	// caller verification
	TODO()

	msgValue := rt.ValueReceived()

	h, st := a.State(rt)
	newTable, ok := actor.BalanceTable_WithAdd(st.EscrowTable(), minerAddr, msgValue)
	if !ok {
		rt.AbortStateMsg("spa.AddBalance: Escrow operation failed.")
	}
	st.Impl().EscrowTable_ = newTable
	UpdateRelease(rt, h, st)
}

func (a *StoragePowerActorCode_I) WithdrawBalance(rt Runtime, minerAddr addr.Address, amountRequested actor.TokenAmount) {

	if amountRequested < 0 {
		rt.AbortArgMsg("spa.WithdrawBalance: negative amount.")
	}

	// caller verification
	TODO()
	msgSender := rt.ImmediateCaller()

	h, st := a.State(rt)

	minerPowerTotal, ok := st._getPowerTotalForMiner(minerAddr)
	if !ok {
		rt.AbortArgMsg("spa.WithdrawBalance: Miner not found.")
	}

	minBalance := st._getPledgeCollateralReq(minerPowerTotal)
	newTable, amountExtracted, ok := actor.BalanceTable_WithExtractPartial(
		st.EscrowTable(), minerAddr, amountRequested, minBalance)
	if !ok {
		rt.AbortStateMsg("spa.WithdrawBalance: Escrow operation failed.")
	}
	st.Impl().EscrowTable_ = newTable

	UpdateRelease(rt, h, st)

	// send funds to sender (pledge collateral)
	rt.SendFunds(msgSender, amountExtracted)
}

func (a *StoragePowerActorCode_I) CreateStorageMiner(
	rt Runtime, workerAddr addr.Address, peerId libp2p.PeerID) addr.Address {

	rt.ValidateImmediateCallerAcceptAnyOfType(actor.BuiltinActorID_Account)

	// ownerAddr := rt.ImmediateCaller()
	msgValue := rt.ValueReceived()

	var newMinerAddr addr.Address
	TODO() // TODO: call InitActor::Exec to create the StorageMinerActor
	panic("")

	// TODO: anything to check here?
	newMinerEntry := &PowerTableEntry_I{
		ActivePower_:   block.StoragePower(0),
		InactivePower_: block.StoragePower(0),
	}

	h, st := a.State(rt)
	newTable, ok := actor.BalanceTable_WithNewAddressEntry(st.EscrowTable(), newMinerAddr, msgValue)
	if !ok {
		panic("Internal error: newMinerAddr (result of InitActor::Exec) already exists in escrow table")
	}
	st.Impl().EscrowTable_ = newTable
	st.PowerTable()[newMinerAddr] = newMinerEntry
	UpdateRelease(rt, h, st)

	return newMinerAddr
}

func (a *StoragePowerActorCode_I) RemoveStorageMiner(rt Runtime) {

	minerAddr := rt.ImmediateCaller()

	// caller verification
	TODO()

	h, st := a.State(rt)

	minerPowerTotal, ok := st._getPowerTotalForMiner(minerAddr)
	if !ok {
		rt.AbortArgMsg("spa.RemoveStorageMiner: miner entry not found in PowerTable.")
	}
	if minerPowerTotal > 0 {
		rt.AbortStateMsg("spa.RemoveStorageMiner: power still remains.")
	}

	minerPledgeBalance, ok := actor.BalanceTable_GetEntry(st.EscrowTable(), minerAddr)
	if !ok {
		rt.AbortArgMsg("spa.RemoveStorageMiner: miner entry not found in escrow.")
	}

	if minerPledgeBalance > 0 {
		rt.AbortStateMsg("spa.RemoveStorageMiner: pledge collateral still remains.")
	}

	delete(st.PowerTable(), minerAddr)

	newTable, ok := actor.BalanceTable_WithDeletedAddressEntry(st.EscrowTable(), minerAddr)
	if !ok {
		panic("Internal error: miner entry in escrow table does not exist")
	}
	st.Impl().EscrowTable_ = newTable

	UpdateRelease(rt, h, st)
}

func (a *StoragePowerActorCode_I) EnsurePledgeCollateralSatisfied(rt Runtime) {

	minerAddr := rt.ImmediateCaller()

	// caller verification
	TODO()

	h, st := a.State(rt)
	pledgeReq := st._getPledgeCollateralReqForMiner(minerAddr)
	UpdateRelease(rt, h, st)

	balanceSufficient, ok := actor.BalanceTable_IsEntrySufficient(st.EscrowTable(), minerAddr, pledgeReq)
	Assert(ok)
	if !balanceSufficient {
		rt.AbortFundsMsg("exitcode.InsufficientPledgeCollateral")
	}

}

// slash pledge collateral for Declared, Detected and Terminated faults
func (a *StoragePowerActorCode_I) SlashPledgeForStorageFault(rt Runtime, affectedPower block.StoragePower, faultType sector.StorageFaultType) {

	minerAddr := rt.ImmediateCaller()

	// caller verification
	TODO()

	h, st := a.State(rt)

	affectedPledge := st._getAffectedPledge(minerAddr, affectedPower)

	TODO() // BigInt arithmetic
	amountToSlash := actor.TokenAmount(
		st._getStorageFaultSlashPledgePercent(faultType) * int(affectedPledge) / 100)

	amountSlashed := st._slashPledgeCollateral(minerAddr, amountToSlash)
	UpdateRelease(rt, h, st)

	rt.SendFunds(addr.BurntFundsActorAddr, amountSlashed)

}

// @param PowerReport with ActivePower and InactivePower
// update miner power based on the power report
func (a *StoragePowerActorCode_I) ProcessPowerReport(rt Runtime, report PowerReport) {

	minerAddr := rt.ImmediateCaller()

	// caller verification
	TODO()

	h, st := a.State(rt)

	powerEntry := st._rtGetPowerEntryOrAbort(rt, minerAddr)

	// keep track of miners larger than minimum miner size before updating the PT
	MIN_MINER_SIZE_STOR := block.StoragePower(0) // TODO: pull in from consts
	if powerEntry.ActivePower() >= MIN_MINER_SIZE_STOR && report.ActivePower() < MIN_MINER_SIZE_STOR {
		st.Impl()._minersLargerThanMin_ -= 1
	}
	if powerEntry.ActivePower() < MIN_MINER_SIZE_STOR && report.ActivePower() >= MIN_MINER_SIZE_STOR {
		st.Impl()._minersLargerThanMin_ += 1
	}

	powerEntry.Impl().ActivePower_ = report.ActivePower()
	powerEntry.Impl().InactivePower_ = report.InactivePower()
	st.Impl().PowerTable_[minerAddr] = powerEntry

	UpdateRelease(rt, h, st)
}

func (a *StoragePowerActorCode_I) ReportConsensusFault(rt Runtime, slasherAddr addr.Address, faultType ConsensusFaultType, proof []block.Block) {
	panic("TODO")

	// Use EC's IsValidConsensusFault method to validate the proof
	// slash block miner's pledge collateral
	// reward slasher

	// include ReportUncommittedPowerFault(cheaterAddr addr.Address, numSectors util.UVarint) as case
	// Quite a bit more straightforward since only called by the cron actor (ie publicly verified)
	// slash cheater pledge collateral accordingly based on num sectors faulted

}

// Surprise is in the storage power actor because it is a singleton actor and surprise helps miners maintain power
// TODO: add Surprise to the cron actor
func (a *StoragePowerActorCode_I) Surprise(rt Runtime) {

	PROVING_PERIOD := 0 // defined in storage_mining, TODO: move constants somewhere else

	// sample the actor addresses
	h, st := a.State(rt)

	randomness := rt.Randomness(rt.CurrEpoch(), 0)
	challengeCount := math.Ceil(float64(len(st.PowerTable())) / float64(PROVING_PERIOD))
	surprisedMiners := st._selectMinersToSurprise(int(challengeCount), randomness)

	UpdateRelease(rt, h, st)

	// now send the messages
	for _, addr := range surprisedMiners {
		// For each miner here send message
		rt.Send(addr, ai.Method_StorageMinerActor_NotifyOfSurprisePoStChallenge, []util.Serialization{}, actor.TokenAmount(0))
	}
}

////////////////////////////////////////////////////////////////////////////////
// Method utility functions
////////////////////////////////////////////////////////////////////////////////

func (st *StoragePowerActorState_I) _rtGetPowerEntryOrAbort(rt Runtime, minerAddr addr.Address) PowerTableEntry {
	powerEntry, found := st.PowerTable()[minerAddr]

	if !found {
		rt.AbortStateMsg("sm._rtGetPowerEntryOrAbort: miner not found in power table.")
	}

	return powerEntry
}

////////////////////////////////////////////////////////////////////////////////
// Dispatch table
////////////////////////////////////////////////////////////////////////////////

func (a *StoragePowerActorCode_I) InvokeMethod(rt Runtime, method actor.MethodNum, params actor.MethodParams) InvocOutput {
	IMPL_FINISH()
	panic("TODO")
}