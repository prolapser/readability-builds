автоматические билды [go-readability](https://github.com/go-shiori/go-readability) при поступлении свежих изменений в оригинальном репо:

⬇️ [linux](https://github.com/prolapser/readability-builds/releases/download/latest/go-readability)

⬇️ [windows](https://github.com/prolapser/readability-builds/releases/download/latest/go-readability.exe)


небольшие отличия от оригинала, вносимые при сборке:

* программа теперь умеет принимать поток stdin а не только путь до файла, что гораздо удобнее для внешних вызовов, использование пайпа и т.д.
* программа может принять строку с html (в windows мало применимо из-за ограничений ее терминала на размер передаваемых аргументов)
* выходные файлы минифицируются
