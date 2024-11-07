SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_NAME='schema_migrations' AND TABLE_SCHEMA = DATABASE();
