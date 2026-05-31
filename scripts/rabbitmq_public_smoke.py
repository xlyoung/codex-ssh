#!/usr/bin/env python3
"""RabbitMQ public endpoint smoke test.

This script performs a real AMQP round trip:
1. Connect to a RabbitMQ endpoint.
2. Declare a temporary queue.
3. Publish a unique message to that queue.
4. Read the message back and verify the payload.
"""

from __future__ import annotations

import argparse
import sys
import time
import uuid

try:
    import pika
    from pika.exceptions import AMQPError
except ImportError as exc:  # pragma: no cover - runtime dependency check
    print(
        "缺少依赖 pika。先执行: python3 -m pip install pika",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="RabbitMQ public smoke test")
    parser.add_argument("--host", required=True, help="RabbitMQ host")
    parser.add_argument("--port", required=True, type=int, help="RabbitMQ port")
    parser.add_argument("--user", required=True, help="RabbitMQ username")
    parser.add_argument("--password", required=True, help="RabbitMQ password")
    parser.add_argument("--vhost", default="/", help="RabbitMQ vhost")
    parser.add_argument(
        "--timeout",
        default=8.0,
        type=float,
        help="socket and blocked-connection timeout in seconds",
    )
    return parser.parse_args()


def build_connection(args: argparse.Namespace) -> pika.BlockingConnection:
    credentials = pika.PlainCredentials(args.user, args.password)
    params = pika.ConnectionParameters(
        host=args.host,
        port=args.port,
        virtual_host=args.vhost,
        credentials=credentials,
        socket_timeout=args.timeout,
        blocked_connection_timeout=args.timeout,
        connection_attempts=1,
        retry_delay=0,
        heartbeat=30,
    )
    return pika.BlockingConnection(params)


def main() -> int:
    args = parse_args()
    message = f"codex-rabbitmq-smoke-{uuid.uuid4()}"
    queue_name = ""

    try:
        print(f"[1/4] 连接 {args.host}:{args.port} vhost={args.vhost} user={args.user}")
        connection = build_connection(args)
        channel = connection.channel()

        print("[2/4] 声明临时队列")
        declared = channel.queue_declare(queue="", exclusive=True, auto_delete=True)
        queue_name = declared.method.queue
        print(f"      queue={queue_name}")

        print("[3/4] 发布测试消息")
        channel.basic_publish(
            exchange="",
            routing_key=queue_name,
            body=message.encode("utf-8"),
        )

        print("[4/4] 回读并校验消息")
        deadline = time.monotonic() + args.timeout
        while time.monotonic() < deadline:
            method, properties, body = channel.basic_get(queue=queue_name, auto_ack=False)
            if method is None:
                time.sleep(0.2)
                continue
            body_text = body.decode("utf-8")
            if body_text != message:
                print(
                    f"消息不匹配: expected={message!r} actual={body_text!r}",
                    file=sys.stderr,
                )
                channel.basic_nack(method.delivery_tag, requeue=False)
                connection.close()
                return 1
            channel.basic_ack(method.delivery_tag)
            connection.close()
            print("SUCCESS: RabbitMQ 发布/消费回路正常")
            return 0

        print("超时: 指定时间内没有读回测试消息", file=sys.stderr)
        connection.close()
        return 1
    except AMQPError as exc:
        print(f"AMQP 失败: {exc}", file=sys.stderr)
        return 1
    except OSError as exc:
        print(f"网络失败: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
