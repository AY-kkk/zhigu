def test_jobs_module_importable():
    from app import jobs

    assert jobs.Conflict is not None
