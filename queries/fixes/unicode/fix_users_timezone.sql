UPDATE Users SET Timezone = REPLACE(Timezone, '\\u0000', '') WHERE Timezone LIKE '%\u0000%';
