// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package persist

import (
	"encoding/json"
	"reflect"
	"testing"

	"scaletail.com/tailcfg"
	"scaletail.com/types/key"
)

func fieldsOf(t reflect.Type) (fields []string) {
	for field := range t.Fields() {
		if name := field.Name; name != "_" {
			fields = append(fields, name)
		}
	}
	return
}

func TestControlServerNoisePinPersistence(t *testing.T) {
	want := &Persist{
		ControlServerNoiseKeyOrigin: "http://control.example:60090",
		ControlServerNoiseKey:       key.NewMachine().Public(),
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Persist
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !want.Equals(&got) {
		t.Fatalf("JSON round trip = %#v, want %#v", &got, want)
	}
	clone := got.Clone()
	if !got.Equals(clone) {
		t.Fatal("Clone did not preserve the control server Noise pin")
	}
	view := clone.View()
	if view.ControlServerNoiseKeyOrigin() != want.ControlServerNoiseKeyOrigin || view.ControlServerNoiseKey() != want.ControlServerNoiseKey {
		t.Fatal("PersistView did not expose the control server Noise pin")
	}
}

func TestPersistEqual(t *testing.T) {
	persistHandles := []string{"PrivateNodeKey", "OldPrivateNodeKey", "UserProfile", "NetworkLockKey", "NodeID", "AttestationKey", "ControlServerNoiseKeyOrigin", "ControlServerNoiseKey", "DisallowedTKAStateIDs"}
	if have := fieldsOf(reflect.TypeFor[Persist]()); !reflect.DeepEqual(have, persistHandles) {
		t.Errorf("Persist.Equal check might be out of sync\nfields: %q\nhandled: %q\n",
			have, persistHandles)
	}

	k1 := key.NewNode()
	nl1 := key.NewNLPrivate()
	controlKey1 := key.NewMachine().Public()
	controlKey2 := key.NewMachine().Public()
	tests := []struct {
		a, b *Persist
		want bool
	}{
		{nil, nil, true},
		{nil, &Persist{}, false},
		{&Persist{}, nil, false},
		{&Persist{}, &Persist{}, true},

		{
			&Persist{PrivateNodeKey: k1},
			&Persist{PrivateNodeKey: key.NewNode()},
			false,
		},
		{
			&Persist{PrivateNodeKey: k1},
			&Persist{PrivateNodeKey: k1},
			true,
		},

		{
			&Persist{OldPrivateNodeKey: k1},
			&Persist{OldPrivateNodeKey: key.NewNode()},
			false,
		},
		{
			&Persist{OldPrivateNodeKey: k1},
			&Persist{OldPrivateNodeKey: k1},
			true,
		},

		{
			&Persist{UserProfile: tailcfg.UserProfile{
				ID: tailcfg.UserID(3),
			}},
			&Persist{UserProfile: tailcfg.UserProfile{
				ID: tailcfg.UserID(3),
			}},
			true,
		},
		{
			&Persist{UserProfile: tailcfg.UserProfile{
				ID: tailcfg.UserID(3),
			}},
			&Persist{UserProfile: tailcfg.UserProfile{
				ID:          tailcfg.UserID(3),
				DisplayName: "foo",
			}},
			false,
		},
		{
			&Persist{NetworkLockKey: nl1},
			&Persist{NetworkLockKey: nl1},
			true,
		},
		{
			&Persist{NetworkLockKey: nl1},
			&Persist{NetworkLockKey: key.NewNLPrivate()},
			false,
		},
		{
			&Persist{NodeID: "abc"},
			&Persist{NodeID: "abc"},
			true,
		},
		{
			&Persist{NodeID: ""},
			&Persist{NodeID: "abc"},
			false,
		},
		{
			&Persist{ControlServerNoiseKeyOrigin: "http://control.example", ControlServerNoiseKey: controlKey1},
			&Persist{ControlServerNoiseKeyOrigin: "http://other.example", ControlServerNoiseKey: controlKey1},
			false,
		},
		{
			&Persist{ControlServerNoiseKeyOrigin: "http://control.example", ControlServerNoiseKey: controlKey1},
			&Persist{ControlServerNoiseKeyOrigin: "http://control.example", ControlServerNoiseKey: controlKey2},
			false,
		},
		{
			&Persist{ControlServerNoiseKeyOrigin: "http://control.example", ControlServerNoiseKey: controlKey1},
			&Persist{ControlServerNoiseKeyOrigin: "http://control.example", ControlServerNoiseKey: controlKey1},
			true,
		},
		{
			&Persist{DisallowedTKAStateIDs: nil},
			&Persist{DisallowedTKAStateIDs: []string{"0:0"}},
			false,
		},
		{
			&Persist{DisallowedTKAStateIDs: []string{"0:1"}},
			&Persist{DisallowedTKAStateIDs: []string{"0:1"}},
			true,
		},
		{
			&Persist{DisallowedTKAStateIDs: []string{}},
			&Persist{DisallowedTKAStateIDs: nil},
			true,
		},
	}
	for i, test := range tests {
		if got := test.a.Equals(test.b); got != test.want {
			t.Errorf("%d. Equals = %v; want %v", i, got, test.want)
		}
	}
}
