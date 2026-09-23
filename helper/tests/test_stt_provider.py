from app.adapters.providers import SUPPORTED_LANGUAGES, choose_language


def test_choose_language_picks_most_probable_supported() -> None:
    probabilities = [("ru", 0.2), ("kk", 0.7), ("en", 0.1)]

    assert choose_language(probabilities, SUPPORTED_LANGUAGES) == "kk"


def test_choose_language_ignores_unsupported_languages() -> None:
    # Типичная ошибка Whisper на короткой казахской фразе — татарский выше
    # казахского; вне набора продукта он не должен побеждать.
    probabilities = [("tt", 0.5), ("ky", 0.2), ("kk", 0.2), ("ru", 0.1)]

    assert choose_language(probabilities, SUPPORTED_LANGUAGES) == "kk"


def test_choose_language_detects_english() -> None:
    probabilities = [("en", 0.9), ("ru", 0.05), ("kk", 0.05)]

    assert choose_language(probabilities, SUPPORTED_LANGUAGES) == "en"


def test_choose_language_falls_back_to_first_supported() -> None:
    probabilities = [("uk", 0.6), ("be", 0.4)]

    assert choose_language(probabilities, SUPPORTED_LANGUAGES) == SUPPORTED_LANGUAGES[0]
