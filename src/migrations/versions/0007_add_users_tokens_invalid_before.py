"""为 users 增加 tokens_invalid_before，用于 logout 全局失效。"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa

revision = "0007_users_tokens_invalid_before"
down_revision = "0006_create_audit_logs"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column(
        "users",
        sa.Column(
            "tokens_invalid_before",
            sa.TIMESTAMP(timezone=True),
            nullable=False,
            server_default=sa.text("to_timestamp(0)"),
        ),
    )


def downgrade() -> None:
    op.drop_column("users", "tokens_invalid_before")

