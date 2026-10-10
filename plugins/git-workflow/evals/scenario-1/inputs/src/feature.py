"""User notification module."""

import smtplib
from email.message import EmailMessage


def send_welcome_email(user_email: str, user_name: str) -> bool:
    """Send a welcome email to a newly registered user.

    Args:
        user_email: Recipient's email address.
        user_name: Display name to greet in the email body.

    Returns:
        True if the email was sent successfully, False otherwise.
    """
    msg = EmailMessage()
    msg["Subject"] = f"Welcome to Acme, {user_name}!"
    msg["From"] = "noreply@acme.example.com"
    msg["To"] = user_email
    msg.set_content(
        f"Hi {user_name},\n\n"
        "Thank you for signing up. Your account is ready.\n\n"
        "— The Acme Team"
    )
    try:
        with smtplib.SMTP("localhost") as server:
            server.send_message(msg)
        return True
    except Exception:
        return False
