# Настройка переменных окружения для Docker

## Использование

### 1. Сборка образа с переменной окружения

```bash
# Сборка с передачей API_BASE через build argument
docker build --build-arg VITE_API_BASE=http://your-api-server:8113 -t insight-app .

# Или с использованием переменной окружения
export API_BASE=http://your-api-server:8113
docker build --build-arg VITE_API_BASE=$API_BASE -t insight-app .
```

### 2. Запуск контейнера

```bash
# Запуск с переменной окружения
docker run -p 3000:80 -e VITE_API_BASE=http://your-api-server:8113 insight-app

# Или с использованием переменной окружения
export API_BASE=http://your-api-server:8113
docker run -p 3000:80 -e VITE_API_BASE=$API_BASE insight-app
```

### 3. Использование docker-compose

Создайте файл `.env` в корне проекта:
```bash
API_BASE=http://your-api-server:8113
```

Затем запустите:
```bash
docker-compose up --build
```

## Примеры значений API_BASE

- `http://localhost:8113` - локальная разработка
- `http://api.example.com:8113` - внешний API сервер
- `https://api.production.com` - продакшн API с HTTPS
- `http://backend:8113` - API сервер в той же Docker сети

## Важные замечания

1. Переменная `VITE_API_BASE` должна быть доступна во время сборки (build time)
2. Если переменная не задана, используется значение по умолчанию: `http://localhost:8113`
3. Для изменения API адреса после сборки потребуется пересборка образа

