package store_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/francisguimond/kairos/client"
	"github.com/francisguimond/kairos/internal/store"
)

// The kairos client (client/) is a separate module and cannot import
// internal/store, so it re-declares the wire types. These tests guard against
// the two copies drifting apart: any rename, retag, reorder, type change, or
// added/removed field on either side fails CI.

func TestClientWireTypesMatch(t *testing.T) {
	cases := []struct {
		name   string
		server reflect.Type
		client reflect.Type
	}{
		{"Schedule", reflect.TypeOf(store.Schedule{}), reflect.TypeOf(client.Schedule{})},
		{"CreateScheduleInput", reflect.TypeOf(store.CreateScheduleInput{}), reflect.TypeOf(client.CreateScheduleInput{})},
		{"UpdateScheduleInput", reflect.TypeOf(store.UpdateScheduleInput{}), reflect.TypeOf(client.UpdateScheduleInput{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := tc.client.NumField(), tc.server.NumField(); got != want {
				t.Fatalf("field count: client has %d, store has %d", got, want)
			}
			for i := range tc.server.NumField() {
				sf := tc.server.Field(i)
				cf := tc.client.Field(i)
				if sf.Name != cf.Name {
					t.Errorf("field %d: name client=%q store=%q", i, cf.Name, sf.Name)
				}
				if sf.Tag.Get("json") != cf.Tag.Get("json") {
					t.Errorf("field %q: json tag client=%q store=%q",
						sf.Name, cf.Tag.Get("json"), sf.Tag.Get("json"))
				}
				if st, ct := normType(sf.Type), normType(cf.Type); st != ct {
					t.Errorf("field %q: type client=%q store=%q", sf.Name, ct, st)
				}
			}
		})
	}
}

// TestClientStatusValuesMatch guards the Status enum string values, which are
// what actually travel over the wire.
func TestClientStatusValuesMatch(t *testing.T) {
	pairs := []struct {
		server store.Status
		client client.Status
	}{
		{store.StatusActive, client.StatusActive},
		{store.StatusPaused, client.StatusPaused},
		{store.StatusCompleted, client.StatusCompleted},
		{store.StatusDeleted, client.StatusDeleted},
	}
	for _, p := range pairs {
		if string(p.server) != string(p.client) {
			t.Errorf("status mismatch: store=%q client=%q", p.server, p.client)
		}
	}
}

// normType renders a reflect.Type with the store/client package qualifiers
// stripped, so equivalently-named types (e.g. store.Status vs client.Status)
// compare equal while genuinely different types do not.
func normType(t reflect.Type) string {
	s := t.String()
	s = strings.ReplaceAll(s, "store.", "")
	s = strings.ReplaceAll(s, "client.", "")
	return s
}
