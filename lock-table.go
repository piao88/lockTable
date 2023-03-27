package lock_table

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

const tableSize = 10000

var table [tableSize]int

type Strength int

const (
	free Strength = iota
	rLock
	xLock
)

type reqTyp int

const (
	readShared reqTyp = iota // readShared acquire rLock
	readWrite
)

type transaction struct {
	seqNum int
	txnId  int
}

type lockState struct {
	mu        sync.Mutex
	isLocked  bool
	strength  Strength
	holder    []*guard
	waitQueue []*guard
}

type request struct {
	index int
	typ   reqTyp
}

type lockTable struct {
	table [tableSize]lockState
}

var lt lockTable

type guard struct {
	req   request
	txnId int
	next  chan struct{}
}

var Dependence map[int]*[]int

func (l *lockState) updateDependence() {
	holderTxns := make([]int, 0)
	for i := range l.holder {
		holderTxns = append(holderTxns, l.holder[i].txnId)
	}

	for i := range l.waitQueue {
		Dependence[l.waitQueue[i].txnId] = &holderTxns
	}
}

func (l *lockState) addDependence(pusher int) {
	pushees := make([]int, 0)
	for i := range l.holder {
		pushees = append(pushees, l.holder[i].txnId)
	}
	Dependence[pusher] = &pushees
}

func detectDeadLocks(
	cur int, target int, seen map[int]struct{},
) (string, bool) {
	// init lazy.
	if Dependence == nil {
		Dependence = make(map[int]*[]int)
	}

	if _, ok := Dependence[cur]; !ok {
		return "", false
	}

	if _, ok := seen[cur]; ok {
		return "", false
	}

	seen[cur] = struct{}{}
	for _, txnPush := range *Dependence[cur] {
		if txnPush == target {
			return "->" + fmt.Sprint(txnPush), true
		}

		if chain, ok := detectDeadLocks(txnPush, target, seen); ok {
			return "->" + fmt.Sprint(txnPush) + chain, true
		}

	}
	delete(seen, cur)
	return "", false
}

func (g *guard) waitOnAndDeadLockDetect() error {
	timer := time.NewTimer(time.Second * 1)
	for {
		select {
		case <-g.next:
			return nil
		case <-timer.C:
			seen := make(map[int]struct{})
			if chain, hasDeadLock := detectDeadLocks(g.txnId, g.txnId, seen); hasDeadLock {
				info := "dependency cycle detected " + fmt.Sprint(g.txnId) + chain
				err := fmt.Errorf(info)
				fmt.Println(info)
				return err
			}
			return nil
		default:
			// do nothing
		}
	}

}

func worker(txnId int) error {
	requests := [4]request{}
	readIndex := rand.Int()
	for i := 0; i < 3; i++ {
		requests[i].typ = readShared
		requests[i].index = (readIndex + i) % tableSize
	}
	requests[3].typ = readWrite
	requests[3].index = rand.Int() % tableSize

	total := 0
	for _, req := range requests {
		g := &guard{
			req:   req,
			txnId: txnId,
			next:  make(chan struct{}),
		}
		if err := lt.sequence(g); err != nil {
			return err
		}
		lt.acquireLock(g)

		// execute read or write
		if req.typ == readShared {
			total += table[req.index]
		} else {
			table[req.index] = total
		}

		lt.releaseLock(g)

	}
	return nil
}

func NewLockTable() lockTable {
	return lockTable{}
}

func (lt *lockTable) getLockState(index int) lockState {
	return lt.table[index]
}

func (l *lockState) isEmpty() bool {
	return len(l.waitQueue) == 0
}

func (l *lockState) insertWaitQueue(g *guard) {
	l.waitQueue = append(l.waitQueue, g)
}

func (l *lockState) shareHolder(g *guard) {
	l.holder = append(l.holder, g)
}

// return whether waiting.
func (l *lockState) tryActiveWait(g *guard) bool {
	if !l.isEmpty() {
		return false
	}
	//if l.isLocked
	if l.strength == rLock {
		if g.req.typ == readShared {
			return false
		} else if g.req.typ == readWrite {
			// locked by self and no other txn shared.
			if len(l.holder) == 1 && l.holder[0].txnId == g.txnId {
				return false
			}
			l.insertWaitQueue(g)
			l.addDependence(g.txnId)
			return true
		}
		// lockStrength is xLock
	} else {
		// locked by self.
		if len(l.holder) > 0 && l.holder[0].txnId == g.txnId {
			return false
		}
		l.insertWaitQueue(g)
		l.addDependence(g.txnId)
		return true
	}
	panic("error")
}

func (lt *lockTable) sequence(g *guard) error {
	l := lt.table[g.req.index]
	l.mu.Lock()

	// should wait?
	if !l.tryActiveWait(g) {
		return nil
	}

	fmt.Printf("txn %d waiting\n", g.txnId)
	l.addDependence(g.txnId)
	l.mu.Unlock()
	return g.waitOnAndDeadLockDetect()
}

func (lt *lockTable) acquireLock(g *guard) {
	l := lt.getLockState(g.req.index)
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.waitQueue) > 0 {
		if l.waitQueue[0] == g {
			// remove from waitQueue
			l.waitQueue = append(l.waitQueue[:0], l.waitQueue[1:]...)
		}
	}

	if !l.isLocked {
		l.isLocked = true
		if g.req.typ == readShared {
			l.strength = rLock
		} else {
			l.strength = xLock
		}
		l.holder = append(l.holder, g)
		l.updateDependence()
		return
	}
	if l.isLocked && l.holder[0].txnId != g.txnId {
		panic("lock table bug")
	}

	if l.strength == rLock && g.req.typ == readShared {
		l.shareHolder(g)
		l.updateDependence()
		return
	}

	if l.strength == rLock && g.req.typ == readWrite && len(l.holder) == 1 && l.holder[0].txnId == g.txnId {
		// shared lock only owned by self, lock upgrade.
		l.strength = xLock
		// l.updateDependence()
		return
	}

	panic("lock table unexpected bug")

}

func (g *guard) notify() {
	g.next <- struct{}{}
}

func (lt *lockTable) releaseLock(g *guard) {
	l := lt.getLockState(g.req.index)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.strength == xLock {
		l.strength = free
		l.holder = nil
		l.isLocked = false
		if len(l.waitQueue) > 0 {
			l.waitQueue[0].notify()
		}
		return
	}
	// l.strength == rLock
	for i := range l.holder {
		if l.holder[i] == g {
			l.holder = append(l.holder[:i], l.holder[i+1:]...)
		}
	}
	if len(l.holder) == 0 {
		l.strength = free
		l.isLocked = false
		if len(l.waitQueue) > 0 {
			l.waitQueue[0].notify()
		}
		return
	}
	if len(l.holder) == 1 {
		// Notify txn in waitQueue which owned shared lock by oneself.
		for i := range l.waitQueue {
			if l.waitQueue[i].txnId == l.holder[0].txnId {
				l.waitQueue[i].notify()
			}
		}
	}
	return
}
