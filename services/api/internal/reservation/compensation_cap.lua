-- PH2-23 counter cap: enforce that the advisory remaining counter is never
-- above DB-derived remaining capacity. The compensation worker runs this
-- periodically as a drift guard — if some Redis state was reconstructed
-- (key eviction, hand-edit, replica gap), this re-anchors the counter to
-- the truth.
--
-- KEYS[1] = remaining counter
-- KEYS[2] = drift marker (string, short TTL, observability only)
-- ARGV[1] = capacity (DB-derived)
-- ARGV[2] = drift marker ttl seconds
--
-- returns one of: 'no_counter' (nothing to cap), 'unchanged', 'capped'.

if redis.call('EXISTS', KEYS[1]) == 0 then
    return 'no_counter'
end
local current = tonumber(redis.call('GET', KEYS[1]))
local capacity = tonumber(ARGV[1])
if not current or not capacity or capacity < 0 then
    return 'no_counter'
end
if current > capacity then
    redis.call('SET', KEYS[1], capacity)
    local ttl = tonumber(ARGV[2])
    if ttl and ttl > 0 then
        redis.call('SET', KEYS[2], tostring(current - capacity), 'EX', ttl)
    end
    return 'capped'
end
return 'unchanged'
