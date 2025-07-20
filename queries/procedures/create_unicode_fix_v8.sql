DROP PROCEDURE IF EXISTS CleanUnicodeEscapes;

CREATE PROCEDURE CleanUnicodeEscapes(IN table_name VARCHAR(64), IN column_name VARCHAR(64))
BEGIN
    DECLARE done INT DEFAULT FALSE;
    DECLARE changes_made INT DEFAULT 1;
    DECLARE sql_statement TEXT;
    
    -- Keep running until no more changes are made
    WHILE changes_made > 0 DO
        SET changes_made = 0;
        
        -- Remove any occurrence of \u0000 with any number of preceding backslashes
        SET sql_statement = CONCAT(
            'UPDATE `', table_name, '` ',
            'SET `', column_name, '` = REGEXP_REPLACE(`', column_name, '`, ''\\\\\\\\+u0000'', '''') ',
            'WHERE `', column_name, '` REGEXP ''\\\\\\\\+u0000'''
        );
        
        SET @sql = sql_statement;
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
        
        SET changes_made = ROW_COUNT();
        
        -- Also handle the case without extra backslashes
        SET sql_statement = CONCAT(
            'UPDATE `', table_name, '` ',
            'SET `', column_name, '` = REPLACE(`', column_name, '`, ''\\u0000'', '''') ',
            'WHERE `', column_name, '` LIKE ''%\\u0000%'''
        );
        
        SET @sql = sql_statement;
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
        
        SET changes_made = changes_made + ROW_COUNT();
    END WHILE;
END;
