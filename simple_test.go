package lock_table

import (
	"fmt"
	"testing"
)

func TestDependencyCycle(t *testing.T) {
	Dependence = make(map[int]*[]int)
	Dependence[1] = &[]int{2}
	Dependence[2] = &[]int{1}
	seen := map[int]struct{}{}
	chain, hasDeadLock := detectDeadLocks(1, 1, seen)
	if hasDeadLock {
		info := "dependency cycle detected " + fmt.Sprint(1) + chain
		fmt.Println(info)
	}
}
