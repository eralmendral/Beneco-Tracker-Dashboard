DELETE FROM facebook_reports AS unmapped
WHERE unmapped.feeder = 'UNMAPPED'
  AND EXISTS (
    SELECT 1
    FROM facebook_reports AS mapped
    WHERE mapped.source_id = unmapped.source_id
      AND mapped.feeder <> 'UNMAPPED'
  );
