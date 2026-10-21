package session

import (
	"net"
	"sync"
	"testing"
)

func TestPutAndLookup(t *testing.T) {
	store := NewStore()
	sess := &Session{
		LocalSEID:    1,
		RemoteSEID:   2,
		UplinkTEID:   3,
		DownlinkTEID: 4,
		UEIP:         net.ParseIP("10.60.0.5"),
	}

	if err := store.Put(sess); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}

	got, ok := store.GetByLocalSEID(1)
	if !ok || got != sess {
		t.Errorf("GetByLocalSEID(1) = %v, %v; want the stored session", got, ok)
	}
	got, ok = store.GetByRemoteSEID(2)
	if !ok || got != sess {
		t.Errorf("GetByRemoteSEID(2) = %v, %v; want the stored session", got, ok)
	}
	got, ok = store.GetByUplinkTEID(3)
	if !ok || got != sess {
		t.Errorf("GetByUplinkTEID(3) = %v, %v; want the stored session", got, ok)
	}
	got, ok = store.GetByUEIP(net.ParseIP("10.60.0.5"))
	if !ok || got != sess {
		t.Errorf("GetByUEIP() = %v, %v; want the stored session", got, ok)
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1", store.Len())
	}
}

func TestPutRejectsZeroLocalSEID(t *testing.T) {
	store := NewStore()
	if err := store.Put(&Session{RemoteSEID: 9}); err == nil {
		t.Fatal("Put() = nil, want an error")
	}
	if err := store.Put(nil); err == nil {
		t.Fatal("Put(nil) = nil, want an error")
	}
}

func TestLocalAndRemoteSEIDAreDistinct(t *testing.T) {
	store := NewStore()
	if err := store.Put(&Session{LocalSEID: 10, RemoteSEID: 20}); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}

	if _, ok := store.GetByLocalSEID(20); ok {
		t.Error("GetByLocalSEID(20) found a session; local and remote SEIDs must not collide")
	}
	if _, ok := store.GetByRemoteSEID(10); ok {
		t.Error("GetByRemoteSEID(10) found a session; local and remote SEIDs must not collide")
	}
}

func TestPutReplacesStaleIndexes(t *testing.T) {
	store := NewStore()
	old := &Session{LocalSEID: 1, UplinkTEID: 100, UEIP: net.ParseIP("10.60.0.5")}
	if err := store.Put(old); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}

	updated := &Session{LocalSEID: 1, UplinkTEID: 200, UEIP: net.ParseIP("10.60.0.6")}
	if err := store.Put(updated); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}

	if _, ok := store.GetByUplinkTEID(100); ok {
		t.Error("GetByUplinkTEID(100) found a session; the stale index was not cleared")
	}
	if _, ok := store.GetByUplinkTEID(200); !ok {
		t.Error("GetByUplinkTEID(200) found nothing; want the updated session")
	}
	if _, ok := store.GetByUEIP(net.ParseIP("10.60.0.5")); ok {
		t.Error("GetByUEIP(old) found a session; the stale index was not cleared")
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1", store.Len())
	}
}

func TestDelete(t *testing.T) {
	store := NewStore()
	if err := store.Put(&Session{LocalSEID: 7, RemoteSEID: 8, UplinkTEID: 9}); err != nil {
		t.Fatalf("Put() = %v, want nil", err)
	}

	if !store.Delete(7) {
		t.Fatal("Delete(7) = false, want true")
	}
	if store.Delete(7) {
		t.Error("Delete(7) = true, want false for an already removed session")
	}
	if _, ok := store.GetByRemoteSEID(8); ok {
		t.Error("GetByRemoteSEID(8) found a session after Delete")
	}
	if _, ok := store.GetByUplinkTEID(9); ok {
		t.Error("GetByUplinkTEID(9) found a session after Delete")
	}
	if store.Len() != 0 {
		t.Errorf("Len() = %d, want 0", store.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewStore()
	const workers = 8
	const perWorker = 100

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				// SEID 0 is reserved, so start at 1.
				seid := uint32(w*perWorker+i) + 1
				sess := &Session{LocalSEID: seid, RemoteSEID: seid + 1000}
				if err := store.Put(sess); err != nil {
					t.Errorf("Put() = %v, want nil", err)
					return
				}
				store.GetByLocalSEID(seid)
				store.GetByRemoteSEID(seid + 1000)
				store.Len()
				store.Delete(seid)
			}
		}(w)
	}
	wg.Wait()

	if store.Len() != 0 {
		t.Errorf("Len() = %d, want 0", store.Len())
	}
}
