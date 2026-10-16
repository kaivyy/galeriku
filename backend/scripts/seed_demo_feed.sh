#!/usr/bin/env bash
# Seeds one demo creator account and their posted photos (feed content, not
# merchandise) plus the MinIO preview objects that make the reels render.
# Idempotent: existing photos are reused, so re-running does not duplicate.
set -euo pipefail

COMPOSE="docker compose -f /root/galeriku/docker-compose.yml"
API="http://127.0.0.1:6666"
PGq() { $COMPOSE exec -T postgres psql -U galeriku -d galeriku_db -t -A -c "$1"; }

EMAIL="budi@galeriku-test.invalid"
PASS="Passw0rd!23"

# 1. Register or reuse the demo creator (conflict means the account exists).
REG=$(curl -s -X POST "$API/api/v1/auth/register" -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"display_name\":\"Budi Fotografer\"}")
echo "register: $(echo "$REG" | head -c 120)"

USER_ID=$(PGq "SELECT id FROM users WHERE email_normalized = 'budi@galeriku-test.invalid'")
echo "user: $USER_ID"

# 2. Attach the CREATOR capability so the studio surface opens for this account.
CAP_ID=$($COMPOSE exec -T postgres psql -U galeriku -d galeriku_db -t -A -c "SELECT id FROM capabilities WHERE code = 'CREATOR' LIMIT 1")
$COMPOSE exec -T postgres psql -U galeriku -d galeriku_db -t -A -c "INSERT INTO user_capabilities (user_id, capability_id, status) VALUES ('$USER_ID', '$CAP_ID', 'ACTIVE') ON CONFLICT DO NOTHING" > /dev/null

# 3. Seed eight photos: five personal posts (no price row) + three for sale.
PHOTO_IDS=""
for i in 1 2 3 4 5 6 7 8; do
  TOTAL=$(PGq "SELECT COUNT(*) FROM photos WHERE uploader_user_id = '$USER_ID'")
  if [ "$TOTAL" -ge 8 ]; then
    ID=$(PGq "SELECT id FROM photos WHERE uploader_user_id = '$USER_ID' ORDER BY created_at OFFSET $(($i - 1)) LIMIT 1")
  else
    ID=$(PGq "SELECT generate_uuid_v7()")
    PGq "INSERT INTO photos (id, uploader_user_id, status, width_px, height_px, moderation_status)
         VALUES ('$ID', '$USER_ID', 'READY', 1080, 1920, 'APPROVED')" > /dev/null
  fi
  PHOTO_IDS="$PHOTO_IDS $ID"
  if [ "$i" -ge 6 ]; then
    PGq "INSERT INTO photo_prices (photo_id, price_idr) VALUES ('$ID', 45000) ON CONFLICT (photo_id) DO NOTHING" > /dev/null
  fi
done
echo "photos:$PHOTO_IDS"

# 4-5. Generate gradient JPEGs and upload them straight into the vault.
/tmp/seedphotos/gen $PHOTO_IDS


echo "DONE: login with $EMAIL / $PASS"
