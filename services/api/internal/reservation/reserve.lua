-- PH2-22 atomic pre-admission reservation.
--
-- KEYS[1] = cets:v1:resv:{event_id}:remaining   (string integer)
-- KEYS[2] = cets:v1:resv:{event_id}:hold:{idempotency_hash}  (hash)
-- KEYS[3] = cets:v1:resv:{event_id}:pending     (zset; member=idempotency_hash, score=expires_at)
-- KEYS[4] = cets:v1:resv:{event_id}:version     (string integer)
--
-- ARGV[1] = idempotency_hash
-- ARGV[2] = capacity                            (DB-derived remaining at initialization, or -1 to skip probe)
-- ARGV[3] = capacity_version
-- ARGV[4] = ttl_seconds
-- ARGV[5] = now_unix
-- ARGV[6] = actor_hash
-- ARGV[7] = event_id
-- ARGV[8] = reservation_id
--
-- returns { outcome, reservation_id, expires_at, capacity_version }

-- Duplicate: existing hold for this idempotency tuple.
if redis.call('EXISTS', KEYS[2]) == 1 then
    local id = redis.call('HGET', KEYS[2], 'reservation_id') or ''
    local exp = redis.call('HGET', KEYS[2], 'expires_at_unix') or '0'
    local version = redis.call('HGET', KEYS[2], 'capacity_version') or '0'
    return { 'duplicate', id, exp, version }
end

local ttl = tonumber(ARGV[4])
local now = tonumber(ARGV[5])
if not ttl or ttl <= 0 or not now then
    return { 'misconfigured', '', '0', '0' }
end

-- Lazy counter initialization from DB-derived remaining; skip the PostgreSQL
-- probe once the Redis counter already exists.
local rem = redis.call('GET', KEYS[1])
if not rem then
    local capacity = tonumber(ARGV[2])
    if not capacity or capacity < 0 then
        return { 'needs_probe', '', '0', '0' }
    end
    redis.call('SET', KEYS[1], ARGV[2])
    redis.call('SET', KEYS[4], ARGV[3])
    rem = ARGV[2]
end
local current = tonumber(rem) or 0

if current <= 0 then
    local hold_version = redis.call('GET', KEYS[4]) or ARGV[3]
    return { 'exhausted', '', '0', hold_version }
end

local after = redis.call('DECR', KEYS[1])
if after < 0 then
    redis.call('SET', KEYS[1], 0)
    local hold_version = redis.call('GET', KEYS[4]) or ARGV[3]
    return { 'exhausted', '', '0', hold_version }
end

local expires = now + ttl
local hold_version = redis.call('GET', KEYS[4]) or ARGV[3]
redis.call('HSET', KEYS[2],
    'schema_version',   '1',
    'reservation_id',   ARGV[8],
    'event_id',         ARGV[7],
    'actor_hash',       ARGV[6],
    'idempotency_hash', ARGV[1],
    'capacity_version', hold_version,
    'seats',            '1',
    'created_at_unix',  tostring(now),
    'expires_at_unix',  tostring(expires),
    'state',            'reserved')
redis.call('EXPIRE', KEYS[2], ttl)
redis.call('ZADD', KEYS[3], expires, ARGV[1])
return { 'granted', ARGV[8], tostring(expires), hold_version }
