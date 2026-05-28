-- PH2-22 commit: DB confirmed the booking, remove the hold without returning
-- the slot. The remaining counter must NOT be incremented; the DB authority
-- now owns this seat.
--
-- KEYS[1] = hold hash
-- KEYS[2] = pending zset
-- ARGV[1] = idempotency_hash
--
-- returns 1 if a hold was removed, 0 if no hold existed (already drained by
-- TTL or compensation).

if redis.call('EXISTS', KEYS[1]) == 0 then
    return 0
end
redis.call('DEL', KEYS[1])
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
