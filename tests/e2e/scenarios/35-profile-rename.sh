#!/bin/bash
# 35-profile-rename.sh — Profile CRUD, rename, and security
# Migrated from: tests/integration/profile_rename_test.go + security_profiles_test.go

source "$(dirname "$0")/common.sh"

# ─────────────────────────────────────────────────────────────────
start_test "profile: create + rename + verify"

# Create
pt_post /profiles/create '{"name":"e2e-rename-test"}'
assert_ok "create profile"
ORIG_ID=$(echo "$RESULT" | jq -r '.id')

# Rename via PATCH
pt_patch "/profiles/${ORIG_ID}" '{"name":"e2e-rename-test-renamed"}'
assert_ok "rename profile"
NEW_ID=$(echo "$RESULT" | jq -r '.id')
assert_json_eq "$RESULT" '.name' 'e2e-rename-test-renamed'

# Verify new ID works
pt_get "/profiles/${NEW_ID}"
assert_ok "get by new ID"

# Verify old ID is gone
pt_get "/profiles/${ORIG_ID}"
assert_not_ok "old ID returns error"

# Cleanup
pt_delete "/profiles/${NEW_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: PATCH requires ID (not name)"

pt_post /profiles/create '{"name":"e2e-patch-by-name"}'
assert_ok "create"
PATCH_ID=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/e2e-patch-by-name" '{"name":"new-name"}'
assert_http_status "404" "PATCH by name rejected"

# Cleanup
pt_delete "/profiles/${PATCH_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: DELETE requires ID (not name)"

pt_post /profiles/create '{"name":"e2e-delete-by-name"}'
assert_ok "create"
DEL_ID=$(echo "$RESULT" | jq -r '.id')

pt_delete "/profiles/e2e-delete-by-name"
assert_http_status "404" "DELETE by name rejected"

# Cleanup with ID
pt_delete "/profiles/${DEL_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: rename conflict → 409"

pt_post /profiles/create '{"name":"e2e-conflict-a"}'
assert_ok "create A"
ID_A=$(echo "$RESULT" | jq -r '.id')

pt_post /profiles/create '{"name":"e2e-conflict-b"}'
assert_ok "create B"
ID_B=$(echo "$RESULT" | jq -r '.id')

# Try rename A → B's name
pt_patch "/profiles/${ID_A}" '{"name":"e2e-conflict-b"}'
assert_http_status "409" "conflict on duplicate name"

# Cleanup
pt_delete "/profiles/${ID_A}"
pt_delete "/profiles/${ID_B}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: path traversal rejected"

pt_post /profiles/create '{"name":"e2e-traversal"}'
assert_ok "create"
TRAV_ID=$(echo "$RESULT" | jq -r '.id')

pt_patch "/profiles/${TRAV_ID}" '{"name":"../etc/passwd"}'
assert_not_ok "rejects path traversal"

pt_patch "/profiles/${TRAV_ID}" '{"name":"foo/../../../bar"}'
assert_not_ok "rejects nested traversal"

# Cleanup
pt_delete "/profiles/${TRAV_ID}"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: empty name rejected"

pt_post /profiles/create '{"name":""}'
assert_not_ok "rejects empty name"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: path separator rejected"

pt_post /profiles/create '{"name":"profile/subdir"}'
assert_not_ok "rejects forward slash"

pt_post /profiles/create '{"name":"dir\\myprofile"}'
assert_not_ok "rejects backslash"

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile: valid names accepted"

for NAME in "e2e-valid-1" "e2e_valid_2" "e2eVALID3"; do
  pt_post /profiles/create "{\"name\":\"${NAME}\"}"
  assert_ok "create $NAME"
  VALID_ID=$(echo "$RESULT" | jq -r '.id')
  pt_delete "/profiles/${VALID_ID}"
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "profile reset"

# Create a profile
pt_post /profiles -d '{"name":"reset-test-profile"}'
assert_ok "create profile for reset"
PROFILE_ID=$(echo "$RESULT" | jq -r '.id')

# Reset it
pt_post "/profiles/${PROFILE_ID}/reset" ""
assert_ok "reset profile"

# Clean up
pt_delete "/profiles/${PROFILE_ID}"

end_test
