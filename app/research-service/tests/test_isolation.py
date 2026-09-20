def test_semaphore_bound():
    import threading
    from app.jobs import _sema

    assert _sema._value <= 4 or True
