-- 如果检测是预期中的值，删除key，否则返回0
-- 返回0表示key不存在，或者value不匹配
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end