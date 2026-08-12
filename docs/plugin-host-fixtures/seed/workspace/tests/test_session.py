import unittest

from src.session import SessionStore


class SessionStoreTests(unittest.TestCase):
    def test_new_refresh_updates_session(self) -> None:
        store = SessionStore()
        store.apply_refresh("alice", 2)
        self.assertEqual((store.user_id, store.generation), ("alice", 2))

    def test_empty_user_is_rejected(self) -> None:
        store = SessionStore()
        with self.assertRaises(ValueError):
            store.apply_refresh("", 1)


if __name__ == "__main__":
    unittest.main()
