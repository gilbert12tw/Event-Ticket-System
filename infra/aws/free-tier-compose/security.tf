resource "aws_security_group" "alb" {
  name        = "${var.project_name}-alb"
  description = "HTTPS access to Phase 3 demo ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTPS app access"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.public_app_cidrs
  }

  ingress {
    description     = "HTTPS scrape from observability node"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.observability.id]
  }

  ingress {
    description = "HTTP redirect"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = var.public_app_cidrs
  }

  ingress {
    description     = "HTTP redirect from observability node"
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    security_groups = [aws_security_group.observability.id]
  }

  egress {
    description = "ALB to app nodes"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }
}

resource "aws_security_group" "app" {
  name        = "${var.project_name}-app"
  description = "Phase 3 app node access"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "ALB to app"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  ingress {
    description = "SSH allowlist"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.allowlist_cidrs
  }

  egress {
    description = "Outbound HTTPS/HTTP and VPC services"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "observability" {
  name        = "${var.project_name}-observability"
  description = "Phase 3 LGTM/control node access"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "Grafana allowlist"
    from_port   = 3000
    to_port     = 3000
    protocol    = "tcp"
    cidr_blocks = var.allowlist_cidrs
  }

  ingress {
    description = "OTLP from app nodes"
    from_port   = 4317
    to_port     = 4318
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  ingress {
    description = "LGTM internal APIs from app nodes"
    from_port   = 3100
    to_port     = 4040
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  ingress {
    description = "SSH allowlist"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.allowlist_cidrs
  }

  egress {
    description = "Outbound access"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "rds" {
  name        = "${var.project_name}-rds"
  description = "RDS PostgreSQL access from app nodes"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "PostgreSQL from app"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }

  egress {
    description = "No broad egress needed, kept for managed service defaults"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }
}
