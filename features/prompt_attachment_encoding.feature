Feature: Prompt attachments in non-UTF-8 encodings
  A workspace text file may be stored in a legacy encoding - Windows-1251 is the
  usual case on a Russian Windows install. Attaching such a file through the
  composer's @ picker, or naming it as a bare @path inside the prompt text,
  hydrates its content into the prompt: the bytes are decoded to UTF-8 so the
  model reads the real text. Files that already are UTF-8 pass through untouched.

  Scenario: Attaching a Windows-1251 file hydrates readable text
    Given a workspace file "notes.txt" encoded in windows-1251 with content:
      """
      Привет, мир!
      Это заметка в кодировке Windows-1251, сохранённая блокнотом.
      Проверяем, что вложение доходит до модели без ошибок.
      """
    When I attach "notes.txt" to the prompt "посмотри @notes.txt"
    Then the prompt has a resource for "notes.txt"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      Привет, мир!
      Это заметка в кодировке Windows-1251, сохранённая блокнотом.
      Проверяем, что вложение доходит до модели без ошибок.
      """

  Scenario: A bare @mention of a Windows-1251 file hydrates readable text
    Given a workspace file "readme.txt" encoded in windows-1251 with content:
      """
      Описание проекта на русском языке.
      Файл сохранён в однобайтовой кодировке Windows-1251.
      Агент должен увидеть текст, а не сообщение об ошибке.
      """
    When I hydrate the prompt text "прочитай @readme.txt и перескажи"
    Then the prompt has a resource for "readme.txt"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      Описание проекта на русском языке.
      Файл сохранён в однобайтовой кодировке Windows-1251.
      Агент должен увидеть текст, а не сообщение об ошибке.
      """

  Scenario: Attaching a Windows-1251 source file whose bulk is ASCII code
    Given a workspace file "main.go" encoded in windows-1251 with content:
      """
      package main

      import "fmt"

      // Точка входа в программу.
      func main() {
          fmt.Println("hello")
          // Здесь считаем сумму значений.
          total := 0
          for i := 0; i < 10; i++ {
              total += i
          }
          fmt.Println(total)
      }
      """
    When I attach "main.go" to the prompt "разбери @main.go"
    Then the prompt has a resource for "main.go"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      package main

      import "fmt"

      // Точка входа в программу.
      func main() {
          fmt.Println("hello")
          // Здесь считаем сумму значений.
          total := 0
          for i := 0; i < 10; i++ {
              total += i
          }
          fmt.Println(total)
      }
      """

  @windows
  Scenario: Attaching a Windows-1251 file whose only Cyrillic is one short comment
    Given a workspace file "short.go" encoded in windows-1251 with content:
      """
      package main

      func main() {
          // Готово
      }
      """
    When I attach "short.go" to the prompt "проверь @short.go"
    Then the prompt has a resource for "short.go"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      package main

      func main() {
          // Готово
      }
      """

  Scenario: Attaching a UTF-8 file still hydrates it unchanged
    Given a workspace file "utf8.md" encoded in utf-8 with content:
      """
      # Заголовок
      Обычный UTF-8 файл со смешанным текстом and English words.
      """
    When I attach "utf8.md" to the prompt "see @utf8.md"
    Then the prompt has a resource for "utf8.md"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      # Заголовок
      Обычный UTF-8 файл со смешанным текстом and English words.
      """

  Scenario: Attaching a UTF-16 file hydrates readable text without its byte-order mark
    Given a workspace file "utf16.txt" encoded in utf-16le with content:
      """
      Файл, сохранённый редактором в UTF-16 с меткой порядка байтов.
      Метка не должна попасть в текст, который читает модель.
      """
    When I attach "utf16.txt" to the prompt "посмотри @utf16.txt"
    Then the prompt has a resource for "utf16.txt"
    And the resource mime type is "text/plain; charset=utf-8"
    And the resource text is:
      """
      Файл, сохранённый редактором в UTF-16 с меткой порядка байтов.
      Метка не должна попасть в текст, который читает модель.
      """

  Scenario: A UTF-8 file with a byte-order mark loses the mark, not the text
    Given a workspace file "bom.md" encoded in utf-8-bom with content:
      """
      # Заголовок с меткой
      Файл сохранён с BOM, но модель должна получить чистый текст.
      """
    When I attach "bom.md" to the prompt "see @bom.md"
    Then the prompt has a resource for "bom.md"
    And the resource text is:
      """
      # Заголовок с меткой
      Файл сохранён с BOM, но модель должна получить чистый текст.
      """

  Scenario: A bare @mention that lands on a binary file is left as prose
    Given a workspace file "logo.png" holding binary content
    When I hydrate the prompt text "сравни @logo.png с макетом"
    Then the prompt has no resource blocks
    And the prompt text is "сравни @logo.png с макетом"

  Scenario: Explicitly attaching a binary file is refused
    Given a workspace file "logo.png" holding binary content
    When I attach "logo.png" to the prompt "разбери @logo.png" and it fails
    Then the failure says the file is not text
