def hello():
    """Print a hello message."""
    print("Hello, World!")


def hello_name(name: str) -> str:
    """Return a personalized hello message.
    
    Args:
        name: The name to include in the greeting.
        
    Returns:
        A personalized hello message.
    """
    return f"Hello, {name}!"


if __name__ == "__main__":
    hello()
    print(hello_name("Developer"))
