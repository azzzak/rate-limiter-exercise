Решение [тестового задания](https://gist.github.com/m1ome/2a083b0ee3a44f70c079fabba9e5247a).

## Описание

Конструктор *NewRateLimiter* принимает следующие параметры:
* **batchSize** — максимальное количество элементов в батче при достижении которого батч сразу же отправляется на обработку
* **limitNumber** и **limitInterval** — макисмальное число элементов, которое может быть обработано за интервал времени и сам этот интервал; эти лимиты определяются внешним сервисом
* **minDelay** — минимальная задержка между отправкой батчей; рекомендуется устанавливать, чтобы избежать отправки батчей с небольшим числом элементов

Конструктор возвращает инстанс *RateLimiter*, канал для передачи данных и канал для получения ошибок, возникших при процессинге элементов. Для запуска процессинга необходимо вызвать на инстансе метод *Run* в горутине.

**Пример использования:**
```Go
limiter, dataCh, errCh := ratelimiter.NewRateLimiter(10, 500, 
    time.Second*60, time.Millisecond*100)
go r.Run(limiter.Background())
dataCh <- ratelimiter.Item{}
```

Задержка между отправкой данных вычисляется для плавной передачи полностью заполненных батчей в начале каждого нового интервала (параметр **limitInterval**). Если полный батч не набирается, то по достижении таймаута на обработку отправляются элементы, которые собраны в буфере к текущему моменту. 

В случае простоя или отправки частично заполненных батчей задержка постепенно уменьшается. В результате последующие данные будут отправлены на обработку быстрее (в рамках установленных лимитов). Если передан параметр **minDelay**, то задержка не упадет ниже этого значения.