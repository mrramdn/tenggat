ALTER TABLE item DROP CONSTRAINT item_mata_uang_check;
ALTER TABLE item ADD CONSTRAINT item_mata_uang_check
  CHECK (mata_uang IN ('IDR','USD','SGD'));

ALTER TABLE tugas DROP CONSTRAINT tugas_mata_uang_check;
ALTER TABLE tugas ADD CONSTRAINT tugas_mata_uang_check
  CHECK (mata_uang IN ('IDR','USD','SGD'));

ALTER TABLE aktivitas DROP CONSTRAINT aktivitas_uang_lengkap;
ALTER TABLE aktivitas ADD CONSTRAINT aktivitas_uang_lengkap
  CHECK ((jumlah IS NULL) = (mata_uang IS NULL)
    AND (mata_uang IS NULL OR mata_uang IN ('IDR','USD','SGD')));
