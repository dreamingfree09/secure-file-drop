CC = gcc
CFLAGS = -O2 -Wall -Wextra -I.
LDFLAGS = -lcrypto

TARGET = sfd-hash
SRCS = sfd_hash.c sfd_hash_cli.c
OBJS = $(SRCS:.c=.o)

all: $(TARGET)

$(TARGET): $(OBJS)
	$(CC) $(CFLAGS) -o $@ $(OBJS) $(LDFLAGS)

%.o: %.c sfd_hash.h
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -f $(TARGET) $(OBJS)

test: $(TARGET)
	@echo "Testing sfd-hash with sample file..."
	@echo "hello world" > /tmp/sfd-test.txt
	@./$(TARGET) /tmp/sfd-test.txt
	@rm -f /tmp/sfd-test.txt
	@echo "Test passed!"

.PHONY: all clean test
