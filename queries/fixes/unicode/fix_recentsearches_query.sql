UPDATE RecentSearches SET Query = REPLACE(Query, '\u0000', '') WHERE Query LIKE '%\u0000%';
