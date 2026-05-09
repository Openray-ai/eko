package tokenizer

import "github.com/redis/go-redis/v9"

var (
	touchSessionScript = redis.NewScript(`
local meta = redis.call("GET", KEYS[1])
if not meta then
  return false
end
if redis.call("EXISTS", KEYS[2]) == 0 then
  redis.call("DEL", KEYS[1], KEYS[2])
  return false
end
redis.call("PEXPIRE", KEYS[1], ARGV[1])
redis.call("PEXPIRE", KEYS[2], ARGV[1])
return meta
`)

	loadSessionScript = redis.NewScript(`
local meta = redis.call("GET", KEYS[1])
if not meta then
  return {false, false}
end
local payload = redis.call("GET", KEYS[2])
if not payload then
  redis.call("DEL", KEYS[1], KEYS[2])
  return {meta, false}
end
redis.call("PEXPIRE", KEYS[1], ARGV[1])
redis.call("PEXPIRE", KEYS[2], ARGV[1])
return {meta, payload}
`)
)
