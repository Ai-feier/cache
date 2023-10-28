-- 对于 lua 脚本注意判断返回值类型, 最好统一返回 string 或 int
--1. 检查是不是你的锁
--2. 删除
-- KEYS[1] 就是你的分布式锁的key
-- ARGV[1] 就是你预期的存在redis 里面的 value
if redis.call('get', KEYS[1]) == ARGV[1] then
    -- 是我的锁
    return redis.call('del', KEYS[1])
else
    return 0
end 