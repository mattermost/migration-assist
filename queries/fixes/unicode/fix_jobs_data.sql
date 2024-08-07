UPDATE Jobs SET Data = REPLACE(Data, '\u0000', '') WHERE Data LIKE '%\u0000%';
