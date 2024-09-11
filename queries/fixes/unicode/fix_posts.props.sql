UPDATE Posts SET Props = REPLACE(Props, '\u0000', '') WHERE Props LIKE '%\u0000%';
