UPDATE Users SET NotifyProps = REPLACE(NotifyProps, '\u0000', '') WHERE NotifyProps LIKE '%\u0000%';
