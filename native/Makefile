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
	python3 test_hash.py ./$(TARGET)

.PHONY: all clean test
