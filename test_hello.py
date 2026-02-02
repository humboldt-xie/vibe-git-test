import unittest
from hello import hello, hello_name


class TestHello(unittest.TestCase):
    def test_hello_name_returns_correct_greeting(self):
        result = hello_name("Alice")
        self.assertEqual(result, "Hello, Alice!")
    
    def test_hello_name_with_empty_string(self):
        result = hello_name("")
        self.assertEqual(result, "Hello, !")
    
    def test_hello_name_with_special_characters(self):
        result = hello_name("Bob-O'Connor")
        self.assertEqual(result, "Hello, Bob-O'Connor!")


if __name__ == "__main__":
    unittest.main()
