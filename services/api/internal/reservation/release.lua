-- PH2-22 release: DB rejected/rolled back, return the advisory slot.
--
-- KEYS[1] = remaining counter
-- KEYS[2] = hold hash
-- KEYS[3] = pending zset
-- ARGV[1] = idempotency_hash
--
-- returns 1 if a hold was present and the slot was returned, 0 otherwise.
-- The counter is only INCR'd when both the hold existed AND the counter exists,
-- so a stale release after key eviction cannot create a phantom counter.

if redis.call('EXISTS', KEYS[2]) == 0 then
    return 0
end
redis.call('DEL', KEYS[2])
redis.call('ZREM', KEYS[3], ARGV[1])
if redis.call('EXISTS', KEYS[1]) == 1 then
    redis.call('INCR', KEYS[1])
end
return 1
