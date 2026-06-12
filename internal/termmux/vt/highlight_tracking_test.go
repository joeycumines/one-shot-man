package vt

import "testing"

func TestHighlightTracking_Enable(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	if scr.HighlightTracking {
		t.Fatal("HighlightTracking should be false initially")
	}

	v.csi.Dispatch(scr, 'h', []int{1001}, true)

	if !scr.HighlightTracking {
		t.Fatal("DECSET ?1001h: expected HighlightTracking=true")
	}
}

func TestHighlightTracking_Disable(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	v.csi.Dispatch(scr, 'h', []int{1001}, true)
	if !scr.HighlightTracking {
		t.Fatal("precondition: expected HighlightTracking=true")
	}

	v.csi.Dispatch(scr, 'l', []int{1001}, true)
	if scr.HighlightTracking {
		t.Fatal("DECRST ?1001l: expected HighlightTracking=false")
	}
}

func TestHighlightTracking_CoexistWithNormalTracking(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	v.csi.Dispatch(scr, 'h', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingBasic {
		t.Fatalf("DECSET ?1000h: want MouseTrackingBasic, got %d", scr.MouseTracking)
	}
	if scr.HighlightTracking {
		t.Fatal("DECSET ?1000h should not enable HighlightTracking")
	}

	v.csi.Dispatch(scr, 'h', []int{1001}, true)
	if !scr.HighlightTracking {
		t.Fatal("DECSET ?1001h: expected HighlightTracking=true")
	}
	if scr.MouseTracking != MouseTrackingBasic {
		t.Fatalf("DECSET ?1001h should not change MouseTracking, want Basic, got %d", scr.MouseTracking)
	}

	v.csi.Dispatch(scr, 'l', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingNone {
		t.Fatalf("DECRST ?1000l: want MouseTrackingNone, got %d", scr.MouseTracking)
	}
	if !scr.HighlightTracking {
		t.Fatal("DECRST ?1000l should not disable HighlightTracking")
	}

	v.csi.Dispatch(scr, 'l', []int{1001}, true)
	if scr.HighlightTracking {
		t.Fatal("DECRST ?1001l: expected HighlightTracking=false")
	}
}

func TestHighlightTracking_SavedByDECSC(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	scr.HighlightTracking = true
	v.esc.Dispatch(scr, '7')

	if !scr.SavedHighlightTracking {
		t.Fatal("DECSC did not save HighlightTracking=true")
	}

	scr.HighlightTracking = false
	v.esc.Dispatch(scr, '8')

	if !scr.HighlightTracking {
		t.Fatal("DECRC did not restore HighlightTracking=true")
	}
}

func TestHighlightTracking_SavedBy1049(t *testing.T) {
	v := NewVTerm(24, 80)

	v.primary.HighlightTracking = true

	v.csi.Dispatch(v.active, 'h', []int{1049}, true)
	if v.active != v.alternate {
		t.Fatal("should be on alternate screen after DECSET ?1049h")
	}
	if !v.primary.Saved1049HighlightTracking {
		t.Fatal("mode 1049 did not save HighlightTracking=true")
	}

	v.alternate.HighlightTracking = false

	v.csi.Dispatch(v.active, 'l', []int{1049}, true)
	if v.active != v.primary {
		t.Fatal("should be on primary screen after DECRST ?1049l")
	}
	if !v.primary.HighlightTracking {
		t.Fatal("HighlightTracking not restored after DECRST ?1049l")
	}
}
