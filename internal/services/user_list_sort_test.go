package services

import "testing"

func TestNormalizeUserListSort(t *testing.T) {
	col, dir := NormalizeUserListSort("traffic_used", "asc")
	if col != "traffic_used" || dir != "ASC" {
		t.Fatalf("got %s %s", col, dir)
	}
	col, dir = NormalizeUserListSort("TRAFFIC_USED", "DESC")
	if col != "traffic_used" || dir != "DESC" {
		t.Fatalf("got %s %s", col, dir)
	}
	// injection / unknown → default id DESC
	col, dir = NormalizeUserListSort("password;drop", "asc")
	if col != "id" || dir != "ASC" {
		// invalid column falls back to id; order still respects asc when provided
		t.Fatalf("invalid col: %s %s", col, dir)
	}
	col, dir = NormalizeUserListSort("id", "nope")
	if col != "id" || dir != "DESC" {
		t.Fatalf("bad order fallback: %s %s", col, dir)
	}
	col, dir = NormalizeUserListSort("", "")
	if col != "id" || dir != "DESC" {
		t.Fatalf("empty: %s %s", col, dir)
	}
	// whitelist
	for _, k := range []string{"email", "expire_at", "created_at", "plan_id", "group_id", "enable"} {
		c, _ := NormalizeUserListSort(k, "desc")
		if c != k {
			t.Fatalf("want %s got %s", k, c)
		}
	}
}
