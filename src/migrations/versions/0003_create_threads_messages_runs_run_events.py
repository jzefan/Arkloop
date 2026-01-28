"""创建 Phase 1 核心表：threads/messages/runs/run_events。"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0003_threads_runs_events"
down_revision = "0002_orgs_users_memberships"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "threads",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "org_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("orgs.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "created_by_user_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("title", sa.Text(), nullable=True),
        sa.Column(
            "created_at",
            sa.TIMESTAMP(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_threads_org_id", "threads", ["org_id"], unique=False)
    op.create_index("ix_threads_created_by_user_id", "threads", ["created_by_user_id"], unique=False)

    op.create_table(
        "messages",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "thread_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("threads.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("role", sa.Text(), nullable=False),
        sa.Column("content", sa.Text(), nullable=False),
        sa.Column(
            "created_at",
            sa.TIMESTAMP(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_messages_thread_id", "messages", ["thread_id"], unique=False)

    op.create_table(
        "runs",
        sa.Column(
            "id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "org_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("orgs.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "thread_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("threads.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column(
            "created_by_user_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("users.id", ondelete="SET NULL"),
            nullable=True,
        ),
        sa.Column("status", sa.Text(), nullable=False, server_default=sa.text("'running'")),
        sa.Column("next_event_seq", sa.BigInteger(), nullable=False, server_default=sa.text("1")),
        sa.Column(
            "created_at",
            sa.TIMESTAMP(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_runs_org_id", "runs", ["org_id"], unique=False)
    op.create_index("ix_runs_thread_id", "runs", ["thread_id"], unique=False)

    op.create_table(
        "run_events",
        sa.Column(
            "event_id",
            postgresql.UUID(as_uuid=True),
            primary_key=True,
            server_default=sa.text("gen_random_uuid()"),
        ),
        sa.Column(
            "run_id",
            postgresql.UUID(as_uuid=True),
            sa.ForeignKey("runs.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("seq", sa.BigInteger(), nullable=False),
        sa.Column(
            "ts",
            sa.TIMESTAMP(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column("type", sa.Text(), nullable=False),
        sa.Column(
            "data_json",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text("'{}'::jsonb"),
        ),
        sa.Column("tool_name", sa.Text(), nullable=True),
        sa.Column("error_class", sa.Text(), nullable=True),
        sa.UniqueConstraint("run_id", "seq", name="uq_run_events_run_id_seq"),
    )
    op.create_index("ix_run_events_type", "run_events", ["type"], unique=False)
    op.create_index("ix_run_events_tool_name", "run_events", ["tool_name"], unique=False)
    op.create_index("ix_run_events_error_class", "run_events", ["error_class"], unique=False)


def downgrade() -> None:
    op.drop_index("ix_run_events_error_class", table_name="run_events")
    op.drop_index("ix_run_events_tool_name", table_name="run_events")
    op.drop_index("ix_run_events_type", table_name="run_events")
    op.drop_table("run_events")

    op.drop_index("ix_runs_thread_id", table_name="runs")
    op.drop_index("ix_runs_org_id", table_name="runs")
    op.drop_table("runs")

    op.drop_index("ix_messages_thread_id", table_name="messages")
    op.drop_table("messages")

    op.drop_index("ix_threads_created_by_user_id", table_name="threads")
    op.drop_index("ix_threads_org_id", table_name="threads")
    op.drop_table("threads")
