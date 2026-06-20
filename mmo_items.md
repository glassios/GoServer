Для космической MMO лучше всего использовать **разделение на шаблон предмета (ItemDefinition) и экземпляр предмета (ItemInstance)**.

### 1. Справочник типов предметов

```sql
ItemDefinition
--------------
id
name
category        -- resource, material, module, ship, blueprint
stackable       -- можно ли складывать
volume
meta_data       -- JSON с характеристиками
```

Примеры:

| id | name                   | category  |
| -- | ---------------------- | --------- |
| 1  | Iron Ore               | resource  |
| 2  | Refined Iron           | material  |
| 3  | Laser Cannon           | module    |
| 4  | Fighter Ship           | ship      |
| 5  | Laser Cannon Blueprint | blueprint |

---

### 2. Экземпляры предметов

```sql
ItemInstance
------------
id
definition_id
quantity
location_type
location_id
owner_id
state
created_at
```

* `definition_id` → ItemDefinition.id
* `quantity` — размер стака.
* `location_type`:

  * SPACE_CONTAINER
  * SHIP_CARGO
  * STATION_STORAGE
  * PROCESSING
* `location_id` — ID конкретного контейнера/корабля/станции/процесса.
* `owner_id` — владелец.
* `state` — normal, locked, destroyed и т.п.

---

### 3. Универсальные места хранения

```sql
InventoryLocation
-----------------
id
type
parent_id
```

Примеры:

| id | type            | parent_id       |
| -- | --------------- | --------------- |
| 10 | SHIP_CARGO      | ship_id         |
| 20 | STATION_STORAGE | station_id      |
| 30 | SPACE_CONTAINER | null            |
| 40 | PROCESSING      | refinery_job_id |

Тогда `ItemInstance.location_id` указывает сюда.

---

### 4. Производство и исследования

Чертежи хранить отдельно.

```sql
Blueprint
----------
id
result_definition_id
research_level
```

Рецепты:

```sql
BlueprintIngredient
-------------------
blueprint_id
definition_id
quantity
```

Например:

```
Laser Cannon Blueprint
    5 Refined Iron
    2 Crystal
```

---

### 5. Исследования

```sql
ResearchNode
------------
id
unlocks_definition_id
requires_definition_id
research_time
```

Позволяет открывать:

* новые ресурсы;
* новые материалы;
* новые чертежи.

---

### Итоговая схема

```
ItemDefinition
        ↓
ItemInstance
        ↓
InventoryLocation

Blueprint
    ↓
BlueprintIngredient

ResearchNode
```

### Главный принцип

* **ItemDefinition** — что это за предмет.
* **ItemInstance** — где он находится и сколько его.
* **InventoryLocation** — любое место хранения.
* **Blueprint** — как создать предмет.
* **ResearchNode** — как открыть новые возможности.

Это самая практичная структура: она проста, хорошо индексируется и выдерживает огромное количество предметов в MMO.
