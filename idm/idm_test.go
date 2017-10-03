package idm

import (
	"flag"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/testutils"
)

func TestNew(t *testing.T) {
	_, err := New(nil, "", 0, 1)
	if err == nil {
		t.Fatal("Expected failure, but succeeded")
	}

	_, err = New(nil, "myset", 1<<10, 0)
	if err == nil {
		t.Fatal("Expected failure, but succeeded")
	}

	i, err := New(nil, "myset", 0, 10)
	if err != nil {
		t.Fatalf("Unexpected failure: %v", err)
	}
	if i.handle == nil {
		t.Fatal("set is not initialized")
	}
	if i.start != 0 {
		t.Fatal("unexpected start")
	}
	if i.end != 10 {
		t.Fatal("unexpected end")
	}
}

func TestAllocate(t *testing.T) {
	i, err := New(nil, "myids", 50, 52)
	if err != nil {
		t.Fatal(err)
	}

	if err = i.GetSpecificID(49); err == nil {
		t.Fatal("Expected failure but succeeded")
	}

	if err = i.GetSpecificID(53); err == nil {
		t.Fatal("Expected failure but succeeded")
	}

	o, err := i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 50 {
		t.Fatalf("Unexpected first id returned: %d", o)
	}

	err = i.GetSpecificID(50)
	if err == nil {
		t.Fatal(err)
	}

	o, err = i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 51 {
		t.Fatalf("Unexpected id returned: %d", o)
	}

	o, err = i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 52 {
		t.Fatalf("Unexpected id returned: %d", o)
	}

	o, err = i.GetID()
	if err == nil {
		t.Fatalf("Expected failure but succeeded: %d", o)
	}

	i.Release(50)

	o, err = i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 50 {
		t.Fatal("Unexpected id returned")
	}

	i.Release(52)
	err = i.GetSpecificID(52)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUninitialized(t *testing.T) {
	i := &Idm{}

	if _, err := i.GetID(); err == nil {
		t.Fatal("Expected failure but succeeded")
	}

	if err := i.GetSpecificID(44); err == nil {
		t.Fatal("Expected failure but succeeded")
	}
}

func TestAllocateInRange(t *testing.T) {
	i, err := New(nil, "myset", 5, 10)
	if err != nil {
		t.Fatal(err)
	}

	o, err := i.GetIDInRange(6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if o != 6 {
		t.Fatalf("Unexpected id returned. Expected: 6. Got: %d", o)
	}

	if err = i.GetSpecificID(6); err == nil {
		t.Fatalf("Expected failure but succeeded")
	}

	o, err = i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 5 {
		t.Fatalf("Unexpected id returned. Expected: 5. Got: %d", o)
	}

	i.Release(6)

	o, err = i.GetID()
	if err != nil {
		t.Fatal(err)
	}
	if o != 6 {
		t.Fatalf("Unexpected id returned. Expected: 6. Got: %d", o)
	}

	for n := 7; n <= 10; n++ {
		o, err := i.GetIDInRange(7, 10)
		if err != nil {
			t.Fatal(err)
		}
		if o != uint64(n) {
			t.Fatalf("Unexpected id returned. Expected: %d. Got: %d", n, o)
		}
	}

	if err = i.GetSpecificID(7); err == nil {
		t.Fatalf("Expected failure but succeeded")
	}

	if err = i.GetSpecificID(10); err == nil {
		t.Fatalf("Expected failure but succeeded")
	}

	i.Release(10)

	o, err = i.GetIDInRange(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if o != 10 {
		t.Fatalf("Unexpected id returned. Expected: 10. Got: %d", o)
	}

	i.Release(5)

	o, err = i.GetIDInRange(5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if o != 5 {
		t.Fatalf("Unexpected id returned. Expected: 5. Got: %d", o)
	}

	for n := 5; n <= 10; n++ {
		i.Release(uint64(n))
	}

	for n := 5; n <= 10; n++ {
		o, err := i.GetIDInRange(5, 10)
		if err != nil {
			t.Fatal(err)
		}
		if o != uint64(n) {
			t.Fatalf("Unexpected id returned. Expected: %d. Got: %d", n, o)
		}
	}

	for n := 5; n <= 10; n++ {
		if err = i.GetSpecificID(uint64(n)); err == nil {
			t.Fatalf("Expected failure but succeeded for id: %d", n)
		}
	}

	// New larger set
	ul := uint64((1 << 24) - 1)
	i, err = New(nil, "newset", 0, ul)
	if err != nil {
		t.Fatal(err)
	}

	o, err = i.GetIDInRange(4096, ul)
	if err != nil {
		t.Fatal(err)
	}
	if o != 4096 {
		t.Fatalf("Unexpected id returned. Expected: 4096. Got: %d", o)
	}

	o, err = i.GetIDInRange(4096, ul)
	if err != nil {
		t.Fatal(err)
	}
	if o != 4097 {
		t.Fatalf("Unexpected id returned. Expected: 4097. Got: %d", o)
	}

	o, err = i.GetIDInRange(4096, ul)
	if err != nil {
		t.Fatal(err)
	}
	if o != 4098 {
		t.Fatalf("Unexpected id returned. Expected: 4098. Got: %d", o)
	}
}

var (
	size         = uint64(1 << 20)
	iters        = 500
	first        = 1
	last         = 8
	numInstances = last - first + 1
	lowAllocP    = 0.6
	highAllocP   = 0.9
	lowReleaseP  = 0.5
	f            = 20 // for no ds test, do f times base iterations
	pw           = newpTestHandle(true)
	pn           = newpTestHandle(false)
)

// parallel test handle
// need this given go test will run in parallel the
// parallel tests with ds and the parallel tests with no ds
type pTestHandle struct {
	gIdm       *Idm
	ds         datastore.DataStore
	done       chan chan struct{}
	idsMap     map[int][]uint64
	seed       int64
	lowAlloc   int
	highAlloc  int
	lowRelease int
	numIters   int
}

func (p *pTestHandle) getIdm() (*Idm, error) {
	if p.gIdm != nil {
		return p.gIdm, nil
	}
	return getIdm(p.ds)
}

func newpTestHandle(withDatastore bool) *pTestHandle {
	var err error

	t := &pTestHandle{
		done:       make(chan chan struct{}, numInstances-1),
		idsMap:     make(map[int][]uint64, numInstances),
		seed:       time.Now().Unix(),
		lowAlloc:   int(float32(lowAllocP) * float32(iters)),
		highAlloc:  int(float32(highAllocP) * float32(iters)),
		lowRelease: int(float32(lowReleaseP) * float32(iters)),
		numIters:   iters,
	}
	if withDatastore {
		if t.ds, err = testutils.RandomLocalStore("idm"); err != nil {
			panic(err)
		}
	} else {
		// increase limits for fast test
		t.lowAlloc = f * t.lowAlloc
		t.highAlloc = f * t.highAlloc
		t.lowRelease = f * t.lowRelease
		t.numIters = f * t.numIters
		if t.gIdm, err = getIdm(nil); err != nil {
			panic(err)
		}
	}
	return t
}

func getIdm(ds datastore.DataStore) (*Idm, error) {
	idm, err := New(ds, "idm-test", 0, size-1)
	if err != nil {
		return nil, err
	}
	return idm, nil
}

// The function tests parallel use of interleved IDM GetId() and Release() in two scenarios:
// 1. Multiple IDM instances using the the same backend datastore
//    (simulates concurrent requests on multiple docker nodes to components that relay on remote datastore)
// 2. Multiple threads invoking the same IDM instance with in memory store
//    (simulates concurrent requests on same node to components which use in memory store)
//
// During the first 60% and the last 10% of iterations, all instances request ids.
// During the last 50% of iterations, all instances also release ids. Some release beginning from
// the first allocated, some beginning form the last allocated.
//
// At the end, check if the ds is consistent with current allocations
func runParallelTests(t *testing.T, withStore bool, instance int) {
	var (
		p   *pTestHandle
		idm *Idm
		ids []uint64
		err error
	)

	t.Parallel()

	validateParallelParams(t)

	// Point to the right test handle
	if withStore {
		p = pw
	} else {
		p = pn
	}

	// Pass channelfor signalling end
	if instance != first {
		d := make(chan struct{})
		p.done <- d
		defer close(d)
	}

	if idm, err = p.getIdm(); err != nil {
		t.Fatal(err)
	}

	// Request ids for when inside the allocation range and release the oldest/newest when in the release range
	for i := 0; i < p.numIters; i++ {
		if i < p.lowAlloc || i > p.highAlloc {
			id, err := idm.GetID()
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}

		time.Sleep(time.Duration(rand.Intn(1500)) * time.Microsecond)

		if len(ids) > 0 && i > p.lowRelease {
			// some instances remove oldest, some newest
			if instance%2 == 0 {
				idm.Release(ids[0])
				ids = ids[1:]
			} else {
				idm.Release(ids[len(ids)-1])
				ids = ids[:len(ids)-1]
			}
		}
	}

	// store the allocated ids for this instance
	p.idsMap[instance] = ids

	// First instance is in charge of the accounting
	if instance == first {
		// Wait for all instances to be done
		n := 1
		for d := range p.done {
			select {
			case <-d:
				n++
				if n == numInstances {
					close(p.done)
				}
			}
		}

		// Check the total number of allocations
		total := 0
		for _, ids := range p.idsMap {
			total += len(ids)
		}
		idm.handle.Refresh()
		if uint64(total) != (size - idm.handle.Unselected()) {
			t.Fatalf("\nInconsistent number of allocations: %d. Expected: %d. Seed: %d.\n%s",
				total, size-idm.handle.Unselected(), p.seed, idm.handle)
		}

		// Verify allocations
		for _, id := range ids {
			if !idm.handle.IsSet(id) {
				t.Fatalf("\nInconsistent allocation for instance %d: %d. Seed: %d",
					instance, id, p.seed)
			}
		}
	}
}

func TestParallelIdmWStore1(t *testing.T) {
	runParallelTests(t, true, 1)
}

func TestParallelIdmWStore2(t *testing.T) {
	runParallelTests(t, true, 2)
}

func TestParallelIdmWStore3(t *testing.T) {
	runParallelTests(t, true, 3)
}

func TestParallelIdmWStore4(t *testing.T) {
	runParallelTests(t, true, 4)
}

func TestParallelIdmWStore5(t *testing.T) {
	runParallelTests(t, true, 5)
}

func TestParallelIdmWStore6(t *testing.T) {
	runParallelTests(t, true, 6)
}

func TestParallelIdmWStore7(t *testing.T) {
	runParallelTests(t, true, 7)
}

func TestParallelIdmWStore8(t *testing.T) {
	runParallelTests(t, true, 8)
}

func TestParallelIdmNoStore1(t *testing.T) {
	runParallelTests(t, false, 1)
}

func TestParallelIdmNoStore2(t *testing.T) {
	runParallelTests(t, false, 2)
}

func TestParallelIdmNoStore3(t *testing.T) {
	runParallelTests(t, false, 3)
}

func TestParallelIdmNoStore4(t *testing.T) {
	runParallelTests(t, false, 4)
}

func TestParallelIdmNoStore5(t *testing.T) {
	runParallelTests(t, false, 5)
}

func TestParallelIdmNoStore6(t *testing.T) {
	runParallelTests(t, false, 6)
}

func TestParallelIdmNoStore7(t *testing.T) {
	runParallelTests(t, false, 7)
}

func TestParallelIdmNoStore8(t *testing.T) {
	runParallelTests(t, false, 8)
}

func validateParallelParams(t *testing.T) {
	pTest := flag.Lookup("test.parallel")
	if pTest == nil {
		t.Skip("Skipped because test.parallel flag not set")
	}
	numParallel, err := strconv.Atoi(pTest.Value.String())
	if err != nil {
		t.Fatal(err)
	}
	if numParallel < numInstances {
		t.Skip("Skipped because t.parallel was less than ", numInstances)
	}
}
