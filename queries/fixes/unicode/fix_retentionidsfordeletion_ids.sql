UPDATE RetentionIdsForDeletion SET Ids = REPLACE(Ids, '\\u0000', '') WHERE Ids LIKE '%\u0000%';
