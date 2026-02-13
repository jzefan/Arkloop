"""为 jobs 增加 lease_token：用于稳健的 ack/nack。"""

from __future__ import annotations

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0009_jobs_add_lease_token"
down_revision = "0008_create_jobs_table"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column("jobs", sa.Column("lease_token", postgresql.UUID(as_uuid=True), nullable=True))


def downgrade() -> None:
    op.drop_column("jobs", "lease_token")

