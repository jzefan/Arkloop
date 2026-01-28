"""为 messages 增加 org 归属与跨租户一致性约束。"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0004_messages_org_consistency"
down_revision = "0003_threads_runs_events"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("messages", sa.Column("org_id", postgresql.UUID(as_uuid=True), nullable=True))
    op.add_column(
        "messages",
        sa.Column("created_by_user_id", postgresql.UUID(as_uuid=True), nullable=True),
    )

    op.execute(
        "UPDATE messages AS m "
        "SET org_id = t.org_id "
        "FROM threads AS t "
        "WHERE m.thread_id = t.id "
        "AND m.org_id IS NULL"
    )
    op.alter_column("messages", "org_id", nullable=False)

    op.create_unique_constraint("uq_threads_id_org_id", "threads", ["id", "org_id"])
    op.drop_constraint("messages_thread_id_fkey", "messages", type_="foreignkey")

    op.create_foreign_key(
        "fk_messages_org_id_orgs",
        "messages",
        "orgs",
        ["org_id"],
        ["id"],
        ondelete="CASCADE",
    )
    op.create_foreign_key(
        "fk_messages_created_by_user_id_users",
        "messages",
        "users",
        ["created_by_user_id"],
        ["id"],
        ondelete="SET NULL",
    )
    op.create_foreign_key(
        "fk_messages_thread_org",
        "messages",
        "threads",
        ["thread_id", "org_id"],
        ["id", "org_id"],
        ondelete="CASCADE",
    )

    op.create_index(
        "ix_messages_org_id_thread_id_created_at",
        "messages",
        ["org_id", "thread_id", "created_at"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index("ix_messages_org_id_thread_id_created_at", table_name="messages")
    op.drop_constraint("fk_messages_thread_org", "messages", type_="foreignkey")
    op.drop_constraint("fk_messages_created_by_user_id_users", "messages", type_="foreignkey")
    op.drop_constraint("fk_messages_org_id_orgs", "messages", type_="foreignkey")

    op.create_foreign_key(
        "messages_thread_id_fkey",
        "messages",
        "threads",
        ["thread_id"],
        ["id"],
        ondelete="CASCADE",
    )
    op.drop_constraint("uq_threads_id_org_id", "threads", type_="unique")
    op.drop_column("messages", "created_by_user_id")
    op.drop_column("messages", "org_id")

