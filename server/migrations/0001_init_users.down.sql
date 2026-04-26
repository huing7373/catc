-- 回滚 0001_init_users.up.sql：DROP users 表
-- IF EXISTS 防御性写法：down 文件在 schema 部分缺失场景（手工 fix）下也不报错
DROP TABLE IF EXISTS users;
