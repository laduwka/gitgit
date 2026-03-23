# gitgit

Инструмент для управления структурой проектов в GitLab локально.

[English version](README_EN.md)

## Описание

gitgit рекурсивно клонирует и обновляет все проекты в указанной группе GitLab. Использует горутины для параллельной работы с несколькими репозиториями одновременно.

Возможности:
- Рекурсивное клонирование всех проектов группы (включая подгруппы)
- Обновление (`git pull --all`) уже склонированных проектов
- Фильтрация проектов по регулярному выражению
- Параллельная работа с настраиваемым числом воркеров
- Клонирование по SSH (по умолчанию) или HTTPS

## Установка

### Сборка из исходников

```bash
go build -o gitgit ./cmd/main.go
```

### Установка из релизов

Скачайте готовый бинарник со страницы [Releases](https://github.com/laduwka/gitgit/releases). Доступны сборки для Linux, macOS и Windows (amd64/arm64).

### Docker

```bash
docker build . -t gitgit
```

## Использование

### Настройка токена

Токен можно передать через флаг `-token` или через переменную окружения:

```bash
export TOKEN="ваш_gitlab_токен"
```

Токен создаётся в настройках профиля GitLab: **Settings → Access Tokens**.

### Примеры

Клонировать все проекты группы:

```bash
./gitgit -id 567
```

Обновить уже склонированные проекты (повторный запуск):

```bash
./gitgit -id 567
```

Фильтрация по пути проекта:

```bash
./gitgit -id 567 -regex 'backend/services'
```

Увеличить число параллельных воркеров:

```bash
./gitgit -id 567 -workers 8
```

Использовать HTTPS вместо SSH:

```bash
./gitgit -id 567 -http
```

Указать свой GitLab-инстанс:

```bash
./gitgit -id 567 -url https://git.example.com/api/v4
```

URL также можно задать через переменную окружения:

```bash
export URL="https://git.example.com/api/v4"
./gitgit -id 567
```

### Все флаги

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-id` | — | ID группы GitLab (обязательный) |
| `-url` | `https://gitlab.com/api/v4` | URL GitLab API |
| `-token` | `$TOKEN` | Приватный токен GitLab |
| `-data` | `./<id_группы>` | Рабочая директория |
| `-regex` | `.` | Регулярное выражение для фильтрации проектов |
| `-workers` | `4` | Количество параллельных воркеров |
| `-http` | `false` | Клонировать по HTTPS вместо SSH |

## Docker

```bash
docker build . -t gitgit

docker run -v "$PWD:/data:rw" \
  -v "$HOME/.ssh:/.ssh" \
  -v "$SSH_AUTH_SOCK:/.SSH_AUTH_SOCK" \
  -e TOKEN \
  -u "$UID:$UID" \
  --rm gitgit -id 567
```

Алиас для удобства:

```bash
alias gitgit='docker run --rm -v "$PWD:/data:rw" \
  -v "$HOME/.ssh:/.ssh" \
  -v "$SSH_AUTH_SOCK:/.SSH_AUTH_SOCK" \
  -e TOKEN -u "$UID:$UID" gitgit'
```

Пример алиаса для macOS:

```bash
alias gitgit='docker run --rm -it \
  -v /run/host-services/ssh-auth.sock:/ssh-agent \
  -e SSH_AUTH_SOCK="/ssh-agent" \
  -v "$HOME/.ssh/known_hosts:/root/.ssh/known_hosts" \
  -v "$PWD:/data:rw" \
  -e TOKEN gitgit'
```

## Принцип работы

1. Получает список проектов через GitLab API v4 (`/groups/:id/projects`) с пагинацией
2. Фильтрует архивные проекты и применяет регулярное выражение
3. Параллельно обрабатывает проекты: клонирует новые или обновляет существующие
4. Проекты сохраняются в структуре `<рабочая_директория>/<path_with_namespace>`, повторяя иерархию GitLab

## Разработка

```bash
go test ./...                              # запуск тестов
go build -o gitgit ./cmd/main.go               # сборка
```

Релизы создаются автоматически через [GoReleaser](https://goreleaser.com/) при пуше тега `v*`.

## Ссылки

- [GitLab API — Groups](https://docs.gitlab.com/ee/api/groups.html)
- [GitLab API — Projects](https://docs.gitlab.com/ee/api/projects.html)
