-- PH2-23 compensation drop: the DB confirmed (or settled-non-confirmed-yet
-- the slot is committed) — remove the orphaned hold without returning the
-- slot. Equivalent to commit.lua but tolerant of partial state (either the
-- hold OR the pending member may already be gone).
--
-- KEYS[1] = hold hash
-- KEYS[2] = pending zset
-- ARGV[1] = idempotency_hash
--
-- returns 'dropped' if at least one of the two keys was present, 'missing'
-- if neither existed (nothing to do).

local touched = 0
if redis.call('EXISTS', KEYS[1]) == 1 then
    redis.call('DEL', KEYS[1])
    touched = 1
end
if redis.call('ZSCORE', KEYS[2], ARGV[1]) then
    redis.call('ZREM', KEYS[2], ARGV[1])
    touched = 1
end
if touched == 1 then
    return 'dropped'
end
return 'missing'
