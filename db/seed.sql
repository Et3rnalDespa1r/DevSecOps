BEGIN;

INSERT INTO inventory_items (inventory_number, name, condition_description, usable)
VALUES
    ('DEMO-001', 'Настольная игра', 'Полный комплект, без повреждений', true),
    ('DEMO-002', 'Проектор', 'Исправен, кабель питания в комплекте', true),
    ('DEMO-003', 'Колонка', 'Не работает разъём питания, требуется ремонт', false)
ON CONFLICT (inventory_number) DO NOTHING;

COMMIT;
