class SessionStore:
    def __init__(self) -> None:
        self.user_id = ""
        self.generation = 0

    def apply_refresh(self, user_id: str, generation: int) -> None:
        if not user_id:
            raise ValueError("user_id is required")
        # The fixture intentionally lacks the stale-generation guard described
        # by docs/session-contract.md.
        self.user_id = user_id
        self.generation = generation
