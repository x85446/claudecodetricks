---
title: Personal Finance Dashboard
---

# Spending by Category

```sql categories
SELECT
    category,
    sub_category,
    COUNT(*) AS txn_count,
    ROUND(SUM(ABS(price)), 2) AS total_spent,
    MIN(date) AS first_date,
    MAX(date) AS last_date
FROM transactions
WHERE price < 0
  AND is_plumbing = 0
  AND category IS NOT NULL
  AND category != ''
GROUP BY category, sub_category
ORDER BY total_spent DESC
```

<DataTable data={categories} rows=50 />

---

# Spending by Year

```sql yearly
SELECT
    year,
    CASE WHEN company = '-' THEN 'Personal' ELSE company END AS entity,
    COUNT(*) AS txn_count,
    ROUND(SUM(ABS(price)), 2) AS total_spent
FROM transactions
WHERE price < 0
  AND is_plumbing = 0
GROUP BY year, entity
ORDER BY year DESC, total_spent DESC
```

<DataTable data={yearly} rows=50 />

<BarChart data={yearly} x=year y=total_spent series=entity />

---

# Top Merchants

```sql merchants
SELECT
    m.name AS merchant,
    COUNT(t.id) AS txn_count,
    ROUND(SUM(ABS(t.price)), 2) AS total_spent,
    MIN(t.date) AS first_date,
    MAX(t.date) AS last_date
FROM merchants m
JOIN transactions t ON t.merchant_id = m.id
WHERE t.price < 0
GROUP BY m.id
ORDER BY total_spent DESC
LIMIT 50
```

<DataTable data={merchants} rows=50 />

