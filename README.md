# Discord Registration Bot

Этот бот автоматизирует процесс регистрации новых участников на гильдийном Discord-сервере по игре Black Desert Online, предоставляя интерактивный опрос через приватные каналы и автоматически выдавая роли в зависимости от ответов.

## Основные функции

- **Автоматическая регистрация новых участников**
- **Гибкая система вопросов** (настраивается через JSON)
- **Приватные каналы** для каждого пользователя
- **Автоматическое назначение ролей** по результатам регистрации
- **Команды администрирования** для управления процессом
- **Конфигурация через Discord** без перезапуска бота

## Установка и запуск

1. **Требования**:
   - Discord аккаунт с правами администратора на сервере
   - Токен бота Discord ([получить здесь](https://discord.com/developers/applications))

2. **Клонирование репозитория**:
   ```bash
   git clone https://github.com/reinbowARA/DiscordRegistredBot
   cd DiscordRegistredBot
   ```
3. **Запуск через код:**
   1. **Установка зависимостей**:
      ```bash
      go mod tidy
      ```
   2. **Настройка конфигурации**:
       Создайте файл `.env` в корне проекта:
        ```
        DISCORD_BOT_TOKEN=your_discord_bot_token
        ```
        > [!IMPORTANT]
        > Перед эти нужно установить пакет [github.com/joho/godotenv](https://github.com/joho/godotenv) и приписать в коде main.go `godotenv.Load()`
   3. **Запуск бота**:
      ```bash
      go run main.go
      ```
4. **Запуск через докер:**
   1.  **Настройка конфигурации**:
       Создайте файл `.env` в корне проекта:
        ```
        DISCORD_BOT_TOKEN=your_discord_bot_token
        ```
   2. Запускаем Docker контейнер
      ```sh
      docker compose up
      ```
## Конфигурация сервера

Используйте команду `!init` для настройки сервера:

```
!init guild <server_id> - Установить ID сервера
!init role <role_id> - Установить ID роли регистрации
!init category <category_id> - Установить ID категории для каналов
!init channel <channel_id> - Установить ID канала для команд
!init preserved <roles_id> - Установить сохраняемые роли (через запятую)
!init guild_role <role_id> - Установка роли для согильдийцев
!init friend_role <role_id> - Установка роли для друзей
!init load <json file> - Конфигурация через файл
!init show - Показать текущую конфигурацию
```

### Пример файла конфигурации (`config.json`):
```json
{
  "server_id": "123456789012345678",
  "registration_role_id": "987654321098765432",
  "category_id": "5678901234567890",
  "command_channel_id": "135791357913579",
  "guild_role_id" : "1238756172572365126",
  "friend_role_id" : "1232134214721947721"
}
```

## Настройка вопросов

Вопросы настраиваются через файл `questions.json`. Этот файл позволяет создавать сложные формы регистрации с различными типами вопросов, условиями и действиями.

> [!IMPORTANT]
> Файл регистрации должен соответствовать строгой JSON-структуре. Рекомендуется использовать валидатор JSON для проверки перед применением.

---

## Гайд по составлении файла регистрации

### Общая структура файла

Файл регистрации состоит из трёх основных частей:

```json
{
  "version": "1.0",
  "questions": [...],
  "completion": {
    "message": "Сообщение при завершении",
    "actions": [...]
  }
}
```

---

### Version (обязательное)
Указывает версию схемы данных. Текущая версия: `"1.0"`

---

### Questions (обязательное)

Массив вопросов с уникальным `id` для каждого. Каждый вопрос имеет следующие параметры:

#### Основные параметры вопроса:

| Параметр | Тип | Обязательный | Описание |
|----------|-----|--------------|----------|
| `id` | string | ✅ | Уникальный идентификатор вопроса |
| `order` | int | ✅ | Порядковый номер вопроса (для сортировки) |
| `type` | string | ✅ | Тип вопроса (см. ниже) |
| `required` | bool | ✅ | Обязателен ли ответ |
| `text` | string | ✅ | Текст вопроса, который увидит пользователь |
| `next` | object | ✅ | Определение следующего шага |

#### Типы вопросов:

##### 1. `text_input` - Текстовый ввод
Простой текстовый ответ от пользователя.

```json
{
  "id": "nickname",
  "order": 1,
  "type": "text_input",
  "required": true,
  "text": "Как к вам обращаться?",
  "validation": {
    "min_length": 3,
    "max_length": 50
  },
  "next": {
    "type": "static",
    "question_id": "next_question_id"
  }
}
```

##### 2. `single_choice` - Выбор одного варианта
Предоставляет список вариантов, из которых нужно выбрать один.

```json
{
  "id": "status",
  "order": 2,
  "type": "single_choice",
  "required": true,
  "text": "Ваш статус?",
  "options": [
    {
      "id": "1",
      "text": "Уже в гильдии"
    },
    {
      "id": "2",
      "text": "Желаю вступить"
    }
  ],
  "next": {
    "type": "static",
    "question_id": "next_question_id"
  }
}
```

##### 3. `multiple_choice` - Выбор нескольких вариантов
Пользователь может выбрать несколько вариантов ответа.

```json
{
  "id": "interests",
  "order": 3,
  "type": "multiple_choice",
  "required": true,
  "text": "Что вас интересует?",
  "options": [
    {
      "id": "pve",
      "text": "PVEContent"
    },
    {
      "id": "pvp",
      "text": "PVPContent"
    }
  ],
  "next": {
    "type": "static",
    "question_id": "next_question_id"
  }
}
```

##### 4. `number_input` - Числовой ввод
Ввод числового значения с возможностью валидации диапазона.

```json
{
  "id": "age",
  "order": 4,
  "type": "number_input",
  "required": true,
  "text": "Сколько вам лет?",
  "validation": {
    "min_value": 13,
    "max_value": 100
  },
  "next": {
    "type": "static",
    "question_id": "next_question_id"
  }
}
```

#### Валидация (validation)
Необязательный объект для проверки правильности ответа:

```json
"validation": {
  "min_length": 3,
  "max_length": 100,
  "min_value": 13,
  "max_value": 100,
  "regex": "^[a-zA-Z]+$"
}
```

---

### Actions (действия при ответе)

Действия выполняются после получения ответа от пользователя. Могут быть использованы в каждом вопросе.

| Тип действия | Описание | Параметры |
|--------------|----------|-----------|
| `assign_role` | Выдать роль пользователю | `role_id` - ID роли |
| `save_answer` | Сохранить ответ | `field` - имя поля, `value` - значение |
| `change_nickname` | Изменить никнейм | `format` - формат ника |

#### Пример использования действий:

```json
{
  "id": "character_name",
  "order": 1,
  "type": "text_input",
  "required": true,
  "text": "Напишите вашу фамилию из игры и в скобках как к вам обращаться",
  "actions": [
    {
      "type": "save_answer",
      "field": "user_name",
      "storage": "permanent",
      "value": "@input"
    },
    {
      "type": "change_nickname",
      "format": "{value}"
    }
  ],
  "next": {
    "type": "static",
    "question_id": "next_question"
  }
}
```

#### Плейсхолдеры для действий:

| Плейсхолдер | Описание |
|-------------|----------|
| `@input` | Введённый пользователем текст |
| `@selected.id` | ID выбранного варианта (для choice типов) |
| `@selected.text` | Текст выбранного варианта |
| `@selected.role_id` | Role ID выбранного варианта |
| `{field_name}` | Значение, сохранённое через `save_answer` |

---

### Условные переходы (Conditional)

Вместо линейного порядка можно использовать условия для динамического определения следующего вопроса:

```json
{
  "id": "experience",
  "order": 5,
  "type": "single_choice",
  "required": true,
  "text": "Ваш игровой опыт?",
  "options": [
    {"id": "newbie", "text": "Новичок"},
    {"id": "veteran", "text": "Ветеран"}
  ],
  "next": {
    "type": "conditional",
    "conditions": [
      {
        "if": {
          "field": "experience",
          "operator": "equals",
          "value": "newbie"
        },
        "question_id": "newbie_questions"
      },
      {
        "if": {
          "field": "experience",
          "operator": "equals",
          "value": "veteran"
        },
        "question_id": "veteran_questions"
      }
    ],
    "default": "general_questions"
  }
}
```

#### Операторы условий:

| Оператор | Описание |
|----------|----------|
| `equals` | Равно |
| `not_equals` | Не равно |
| `contains` | Содержит подстроку |

---

### Completion (завершение регистрации)

Действия, выполняемые после ответа на последний вопрос:

```json
"completion": {
  "message": "Спасибо за регистрацию! Администратор свяжется с вами.",
  "actions": [
    {
      "type": "assign_role",
      "role_id": "{guild_role_id}"
    }
  ]
}
```

---

### Полный пример файла регистрации:

```json
{
  "version": "1.0",
  "questions": [
    {
      "id": "name",
      "order": 1,
      "type": "text_input",
      "required": true,
      "text": "Напишите вашу фамилию из игры и в скобках как к вам обращаться",
      "validation": {
        "min_length": 3,
        "max_length": 100
      },
      "actions": [
        {
          "type": "save_answer",
          "field": "user_name",
          "storage": "permanent",
          "value": "@input"
        },
        {
          "type": "change_nickname",
          "format": "{value}"
        }
      ],
      "next": {
        "type": "static",
        "question_id": "guild_status"
      }
    },
    {
      "id": "guild_status",
      "order": 2,
      "type": "single_choice",
      "required": true,
      "text": "Вы уже являетесь членом нашей гильдии?",
      "options": [
        {
          "id": "1",
          "text": "Я уже в гильдии",
          "role_id": "123456789012345678"
        },
        {
          "id": "2",
          "text": "Я из другой гильдии",
          "role_id": "234567890123456789"
        }
      ],
      "actions": [
        {
          "type": "assign_role",
          "role_id": "@selected.role_id"
        }
      ],
      "next": {
        "type": "static",
        "question_id": "end"
      }
    }
  ],
  "completion": {
    "message": "Спасибо за регистрацию!",
    "actions": [
      {
        "type": "assign_role",
        "role_id": "{guild_role_id}"
      }
    ]
  }
}
```

> [!TIP]
> Для проверки корректности JSON используйте онлайн-валидаторы или редакторы с поддержкой JSON Schema.
## Команды администрирования

### Основные команды
- `!init` - Настройка сервера
- `!status` - Статус бота и сервера
- `!help` - Справка по командам

### Управление регистрацией
- `!startRegistred` - Запустить регистрацию для пользователей без роли
- `!stopRegistred` - Принудительно остановить все активные регистрации

### Управление ролями
- `!clsRoles` - Удалить все пользовательские роли (кроме сохраненных)

## Процесс регистрации

1. Новый участник присоединяется к серверу
2. Бот автоматически:
   - Выдает роль "Регистрация"
   - Создает приватный канал
   - Задает вопросы из `questions.json`
3. Пользователь отвечает на вопросы
4. По завершении:
   - Никнейм изменяется на игровой
   - Роль "Регистрация" удаляется
   - Выдается постоянная роль по выбору
   - Приватный канал удаляется через 30 секунд

## Лицензия

Этот проект распространяется под лицензией MIT. Подробнее см. в файле `LICENSE`.

## Поддержка

По вопросам и предложениям обращайтесь:
- Через Issues на GitHub
- В Discord: [reinbow_ara](https://discord.com/users/302859679929729024)
