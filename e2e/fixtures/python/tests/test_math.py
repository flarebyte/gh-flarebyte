import pytest

from flarebyte_python_fixture import add


def test_add():
    assert add(2, 3) == 5


@pytest.mark.skip(reason="fixture exercises skip reporting")
def test_skipped():
    pass
