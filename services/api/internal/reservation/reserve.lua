-- PH2-22 atomic pre-admission reservation.
--
-- KEYS[1] = cets:v1:resv:{event_id}:remaining   (string integer)
-- KEYS[2] = cets:v1:resv:{event_id}:hold:{idempotency_hash}  (hash)
-- KEYS[3] = cets:v1:resv:{event_id}:pending     (zset; member=idempotency_hash, score=expires_at)
--
-- ARGV[1] = idempotency_hash
-- ARGV[2] = capacity                            (DB-derived remaining at initialization)
-- ARGV[3] = capacity_version
-- ARGV[4] = ttl_seconds
-- ARGV[5] = now_unix
-- ARGV[6] = actor_hash
-- ARGV[7] = event_id
-- ARGV[8] = reservation_id
--
-- returns { outcome, reservation_id, expires_at }

-- Duplicate: existing hold for this idempotency tuple.
if redis.call('EXISTS', KEYS[2]) == 1 then
    local id = redis.call('HGET', KEYS[2], 'reservation_id') or ''
    local exp = redis.call('HGET', KEYS[2], 'expires_at_unix') or '0'
    return { 'duplicate', id, exp }
end

local capacity = tonumber(ARGV[2])
local ttl = tonumber(ARGV[4])
local now = tonumber(ARGV[5])
if not capacity or capacity < 0 or not ttl or ttl <= 0 or not now then
    return { 'misconfigured', '', '0' }
end

-- Lazy counter initialization from DB-derived remaining; never above truth.
local rem = redis.call('GET', KEYS[1])
if not rem then
    redis.call('SET', KEYS[1], ARGV[2])
    rem = ARGV[2]
end
local current = tonumber(rem) or 0
if current > capacity then
    current = capacity
    redis.call('SET', KEYS[1], capacity)
end

if current <= 0 then
    return { 'exhausted', '', '0' }
end

local after = redis.call('DECR', KEYS[1])
if after < 0 then
    redis.call('SET', KEYS[1], 0)
    return { 'exhausted', '', '0' }
end

local expires = now + ttl
redis.call('HSET', KEYS[2],
    'schema_version',   '1',
    'reservation_id',   ARGV[8],
    'event_id',         ARGV[7],
    'actor_hash',       ARGV[6],
    'idempotency_hash', ARGV[1],
    'capacity_version', ARGV[3],
    'seats',            '1',
    'created_at_unix',  tostring(now),
    'expires_at_unix',  tostring(expires),
    'state',            'reserved')
redis.call('EXPIRE', KEYS[2], ttl)
redis.call('ZADD', KEYS[3], expires, ARGV[1])
return { 'granted', ARGV[8], tostring(expires) }
