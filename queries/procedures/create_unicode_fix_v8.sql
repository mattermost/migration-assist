DROP PROCEDURE IF EXISTS CleanUnicodeEscapes;

CREATE PROCEDURE CleanUnicodeEscapes(IN table_name VARCHAR(64), IN column_name VARCHAR(64))
BEGIN
    DECLARE changes_made INT DEFAULT 1;
    DECLARE sql_statement TEXT;
    DECLARE max_iterations INT DEFAULT 5;
    
    main_loop: WHILE changes_made > 0 DO
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

        -- Limit the number of iterations
        SET max_iterations = max_iterations - 1;
        IF max_iterations <= 0 THEN
            LEAVE main_loop;
        END IF;
    END WHILE main_loop;
END;
