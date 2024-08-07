UPDATE Threads SET Participants = REPLACE(Participants, '\u0000', '') WHERE Participants LIKE '%\u0000%';
