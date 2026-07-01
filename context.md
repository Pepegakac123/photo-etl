# Kontekst Projektu: Overflow Photo ETL (Asset Manager)

Jesteś Senior Go & HTMX Developerem. Twoim zadaniem jest stworzenie wewnętrznego, lokalnego narzędzia webowego/CLI dla agencji webowej Overflow. Narzędzie ma drastycznie przyspieszyć proces selekcji, sortowania i generowania zdjęć budowlanych/usługowych, które docelowo lądują na stronach WordPress klientów.

Narzędzie opiera się na architekturze "80% automatyzacji, 20% ludzkiej superwizji".

## 1. Założenia Biznesowe i Przepływ Pracy (Workflow)

Aplikacja procesuje foldery ze zdjęciami w następujących krokach:

1. **Ingestion (Pobieranie struktury i kontekstu):** Użytkownik podaje ścieżkę do lokalnego folderu dla danego klienta (pobranego wcześniej z Google Drive). Folder ten zawiera podfoldery nazwane od usług (np. `Abbrucharbeiten`, `Fassadenbau`, `remonty`). **Język tych folderów staje się głównym kontekstem językowym dla danego zlecenia.** Aplikacja mapuje tę strukturę.
2. **AI Vision Sorting (Sortowanie zrzutów):** W głównym katalogu klienta może znajdować się folder ze zrzutami (np. z WhatsAppa). Narzędzie dedukuje jego istnienie szukając w nazwie słów kluczowych (np. "whatsapp", "zrzuty"). Jeśli go nie znajdzie, aplikacja pyta użytkownika, czy taki folder istnieje, pozwalając mu wskazać go z listy. Następnie zdjęcia z tego folderu są wysyłane do modelu AI Vision, który klasyfikuje je do odpowiednich podfolderów usługowych klienta lub odrzuca jako nieprzydatne.
3. **Weryfikacja braków:** Docelowo każda usługa potrzebuje określonej liczby zdjęć. System iteruje po zmapowanych folderach i wyłapuje te, w których występuje deficyt.
4. **Tryb Superwizji (UI):** Użytkownik wchodzi do interfejsu webowego (HTMX). Widzi pasek boczny z listą usług i postępem (np. 2/5). Wybiera usługę i uzupełnia braki, korzystając z trzech metod:
   - **Lokalna Galeria:** Pobranie propozycji z bazy SQLite zawierającej zindeksowane zasoby własne. Ścieżka bazowa to: `/home/kacper/GoogleDrive/overflow-praca/Galeria Zdjęć Usługowych`. 
     *Inteligentne wyszukiwanie:* Foldery w lokalnej galerii mają strukturę hybrydową (np. `ObcyJęzyk_PolskiJęzyk` jak `Badsanierung_Remont łazienki` lub tylko polską jak `Adaptacja Poddasza`). Aplikacja bierze nazwę usługi klienta i w pierwszej kolejności szuka bezpośredniego dopasowania (fuzzy search). Jeśli dopasowanie jest słabe, używa taniego modelu LLM, aby przetłumaczyć nazwę usługi klienta na język polski i szuka dopasowania po polskim członie nazw folderów w galerii. Jeśli system nie jest pewny dopasowania, podpowiada użytkownikowi kilka folderów do ręcznego zatwierdzenia.
   - **Stock (Envato):** Pobranie miniatur z API/Scrapera z obsługą paginacji. Zapytanie do wyszukiwarki korzysta z oryginalnego (klienckiego) języka usługi lub tłumaczenia na angielski, w zależności od wymagań API.
   - **Generowanie AI (Nano Banana):** Aplikacja buduje precyzyjny prompt, łącząc systemowy prompt bazowy z kontekstem usługi (nazwa usługi, kraj klienta oraz krótki, wygenerowany w tle opis tego, na czym polega ta usługa). Cel: wygenerowanie realistycznych, "amatorskich" zdjęć budowlanych (brak twarzy od przodu, brak widocznych logotypów, styl zdjęcia z telefonu robotnika).
5. **Eksport i Integracja z GoPress:** Po zaakceptowaniu zestawu dla usługi, wybrane zdjęcia zewnętrzne (Stock, AI) są kopiowane do wydzielonego katalogu wyjściowego. Narzędzie oznacza źródło pochodzenia pliku w bazie. Następnie zdjęcia są gotowe do przejęcia przez zewnętrzne narzędzie CLI `gopress` (szczegóły: `https://github.com/Pepegakac123/gopress`), które zoptymalizuje je i wgra na WordPressa.

## 2. Architektura i Stos Technologiczny

- **Język:** Go 1.22+
- **Frontend:** HTMX, TailwindCSS (via CDN), szablony `html/template` z wbudowanej biblioteki Go. Zero frameworków SPA (React/Vue/itp.).
- **Backend / Routing:** Wbudowany `net/http` (wzorzec `http.NewServeMux()`). 
- **Baza Danych:** SQLite z użyciem sterownika `modernc.org/sqlite` (brak zależności CGO, co ułatwia kompilację). Baza przechowuje stan aplikacji w trakcie pracy nad danym zleceniem (krótkotrwały cykl życia).

## 3. Parametryzacja i Konfiguracja (config.yaml / .env)

Aplikacja musi wczytywać ustawienia, pozwalając na elastyczną zmianę środowiska pracy. Wymagane parametry to:
- `TARGET_PHOTOS_PER_SERVICE`: Wymagana liczba zdjęć na folder (domyślnie: 5).
- `LOCAL_GALLERY_PATH`: Ścieżka do głównego dysku z prywatną galerią zdjęć (domyślnie: `/home/kacper/GoogleDrive/overflow-praca/Galeria Zdjęć Usługowych`).
- `EXPORT_DIR`: Ścieżka, do której trafiają zaakceptowane zdjęcia ze stocków i AI, gotowe dla narzędzia `gopress`.
- `GOPRESS_CMD_PATH`: Opcjonalna ścieżka do pliku wykonywalnego `gopress`.
- `CONCURRENCY_LIMIT`: Maksymalna liczba jednoczesnych zapytań do zewnętrznych API (np. 5), aby uniknąć rate-limitów.
- **Klucze i Modele API:**
  - `OPENAI_API_KEY` / `AI_VISION_MODEL`: Klucz i model (np. `gpt-4o-mini`) używany do sortowania zdjęć z WhatsAppa, generowania opisów kontekstowych oraz tłumaczenia fraz do wyszukiwania.
  - `ENVATO_API_TOKEN` / `NANO_BANANA_KEY`: Klucze do zewnętrznych dostawców stockowych i generatywnych.
- **Prompty AI (Systemowe):**
  - `VISION_SORTING_PROMPT`: "Jesteś ekspertem budowlanym. Zwróć JSON z przypisaniem zdjęcia do jednej z podanych kategorii. Jeśli zdjęcie to śmieć, zwróć kategorię 'REJECT'."
  - `IMAGE_GENERATION_BASE_PROMPT`: "Zdjęcie musi wyglądać jak zrobione amatorsko, telefonem komórkowym, naturalne oświetlenie na budowie. Złota zasada: brak widocznych twarzy, brak logotypów, brak napisów. Styl surowy i realistyczny."

## 4. Wymagania dotyczące Bazy Danych (Struktura)

Baza SQLite musi solidnie śledzić stan plików w bieżącym projekcie. Minimalny schemat:
- Tabela `services`: `id`, `name` (oryginalna nazwa z folderu klienta), `context_description` (krótki opis wygenerowany przez LLM do poprawy wyników AI), `status` (pending/completed).
- Tabela `photos`: `id`, `service_id`, `file_path`, `source` (enum: `Client`, `LocalGallery`, `Stock`, `AI`), `status` (enum: `pending`, `approved`, `rejected`).

## 5. Wytyczne dla UI (HTMX)

- **Układ:** Dwukolumnowy. Sidebar z listą usług (oryginalna nazwa + licznik `zaakceptowane / wymagane`). Obszar główny to przestrzeń robocza dla wybranej usługi.
- **Akcje:**
  - Kliknięcie usługi w sidebarze ładuje obszar roboczy (`hx-get`, `hx-target`).
  - Przycisk "Załaduj więcej ze stocka" ma doklejać zdjęcia na koniec siatki bez nadpisywania obecnych (`hx-swap="beforeend"`).
  - W trakcie zapytań do AI (Vision, tłumaczenie, generowanie) bezwzględnie musi pojawiać się wskaźnik ładowania (`hx-indicator`).
  - Akceptacja zdjęcia (`hx-post`) musi odświeżać licznik usługi w sidebarze natychmiast (technika HTMX Out-of-Bounds - OOB swap).

## 6. Zasady Kodowania (Idiomatyczne Go)

Jeśli generujesz kod, bezwzględnie przestrzegaj tych zasad:
1. **Struktura projektu:** Podział na pakiety. Punkt wejścia to `cmd/server/main.go`. Logika domenowa w `internal/` (np. `internal/storage`, `internal/vision`, `internal/gallery`, `internal/ui`). Widoki w folderze `views/`.
2. **Współbieżność:** Podczas jednoczesnego szukania zdjęć w bazie lokalnej, na Envato i w generatorze AI, używaj `golang.org/x/sync/errgroup`.
3. **Obsługa Błędów:** Zawsze owijaj błędy, by zapewnić pełny trace: `fmt.Errorf("failed to process %s: %w", serviceName, err)`. Nigdy nie ignoruj błędów (`_`).
4. **Konteksty:** Zawsze propaguj `context.Context` w dół do zapytań DB oraz żądań HTTP.
5. **Kompilacja i Weryfikacja:** Pisz kod czysty, modularny. Kiedy skończysz pisać dany moduł, uruchom w terminalu `go build ./...` oraz zaktualizuj zależności przez `go mod tidy`.
6. Prowadź TTD Test-Driven-Development

## 7. Zasady Działania Agenta (Kluczowe!)

Podczas pracy ze mną nad tym projektem, obowiązują Cię następujące reguły:
- **Pytaj, nie zgaduj:** Jeśli nie jesteś pewien jakiejś funkcjonalności, logiki biznesowej, albo tego jak dany element ma działać – zatrzymaj się i zadaj mi pytanie. Będziemy prowadzić konwersację iteracyjnie, aż ustalimy optymalne podejście.
- **Wykorzystuj Narzędzia (MCP):** Aktywnie korzystaj z dostępnych narzędzi MCP oraz przeglądarki, aby przeszukiwać dokumentację wymaganych bibliotek Go i API, zanim napiszesz kod.
- **Szczerość kompetencji:** Jeśli nie wiesz, jak dokładnie działa konkretne API (np. Envato lub Nano Banana), nie udawaj, że wiesz i nie zmyślaj endpointów. Poinformuj mnie o tym, a ja dostarczę Ci odpowiednią dokumentację lub linki.
- **Brak założeń co do AI:** Nie zakładaj z góry, jakich konkretnie modeli AI użyjemy do każdego z zadań, chyba że jest to podane w konfiguracji. Przed implementacją zapytaj o preferowany model do danego zadania (np. tłumaczenie vs. generowanie).
- **Proaktywność:** Jeśli podczas pisania kodu wpadniesz na pomysł usprawnienia (np. optymalizacja bazy, lepszy caching, ciekawsze użycie HTMX), zatrzymaj się, zaproponuj to rozwiązanie i poczekaj na moją decyzję.
