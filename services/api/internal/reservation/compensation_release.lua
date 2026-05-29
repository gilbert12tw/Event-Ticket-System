-- PH2-23 compensation release: return the advisory slot for an orphaned
-- pending hold (DB never confirmed). Safe for the compensation worker
-- because it only acts while the pending-set member still exists, and it
-- only increments the counter when the counter is already initialized —
-- so a stale sweep after key eviction cannot create a phantom counter.
--
-- KEYS[1] = remaining counter
-- KEYS[2] = hold hash
-- KEYS[3] = pending zset
-- ARGV[1] = idempotency_hash
-- ARGV[2] = capacity (DB-derived; counter is capped at this value on release)
--
-- returns one of: 'released', 'released_missing_hold', 'missing_pending'

local pending_score = redis.call('ZSCORE', KEYS[3], ARGV[1])
if not pending_score then
    -- Pending member was already drained by application Release or by
    -- a previous compensation tick. If a stale hold still exists, drop it.
    if redis.call('EXISTS', KEYS[2]) == 1 then
        redis.call('DEL', KEYS[2])
    end
    return 'missing_pending'
end

if redis.call('EXISTS', KEYS[2]) == 0 then
    -- Hold expired via TTL but pending member wasn't cleaned up. The TTL
    -- removed only the hold hash; reserve.lua already decremented the
    -- counter, so this pending member is still the idempotency guard that
    -- lets compensation return the slot at most once.
    redis.call('ZREM', KEYS[3], ARGV[1])
    if redis.call('EXISTS', KEYS[1]) == 1 then
        local after = redis.call('INCR', KEYS[1])
        local capacity = tonumber(ARGV[2])
        if capacity and after > capacity then
            redis.call('SET', KEYS[1], capacity)
        end
    end
    return 'released_missing_hold'
end

redis.call('DEL', KEYS[2])
redis.call('ZREM', KEYS[3], ARGV[1])
if redis.call('EXISTS', KEYS[1]) == 1 then
    local after = redis.call('INCR', KEYS[1])
    local capacity = tonumber(ARGV[2])
    if capacity and after > capacity then
        redis.call('SET', KEYS[1], capacity)
    end
end
return 'released'
