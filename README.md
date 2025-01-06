# Telegram Poll Bot

Telegram Poll Bot is a Go-based bot designed to facilitate book club meetings by organizing polls to select books. Users can suggest books, provide book details, and participate in a voting process to choose the next book for discussion.

## Features

- **User Subscription:** Users can subscribe to the bot to participate in polls.
- **Book Suggestion:** Subscribers can suggest books with details like title, author, and description.
- **Poll Management:** The bot creates polls in group chats, allowing members to vote on suggested books.
- **Automatic Poll Closure:** Automatically closes polls after a configurable time and announces the winner.
- **Persistent Data Storage:** User subscriptions and book suggestions are stored persistently in a JSON database.

## Prerequisites

- Go 1.18 or higher
- Telegram Bot API Key - **Must be placed as telegrammApiKey env variable** (create a bot using [BotFather](https://core.telegram.org/bots#botfather))
- A configured Telegram group to use the bot - **Must be places as groupId env variable**

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/telegram-poll-bot.git
   cd telegram-poll-bot
   ```

2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Set up configuration:
   - Create a `config.json` file in the `config` directory with the following structure:
     ```json
     {
       "TimeToGatherBooks": 3600,
       "NotifyBeforeGathering": 300,
       "TimeForTelegramPoll": 1800,
       "NotifyBeforePoll": 300
     }
     ```

4. Run the bot:
   ```bash
   go run .
   ```

## Usage

- **/subscribe**: Subscribe to the bot to participate in future polls.
- **/start_vote**: Start a new book gathering and initiate the voting process.
- **/skip**: Skip suggesting a book during the gathering phase.

## Directory Structure

- `bot.go`: Main bot implementation
- `helpers.go`: Utility functions for poll and book management
- `poll.go`: Poll-related structures and logic
- `user.go`: User subscription management and database handling
- `config/`: Contains configuration files
- `db/`: Stores subscription and book data in JSON format

## Testing

Run unit tests with the following command:
```bash
go test ./...
```

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository.
2. Create a new branch for your feature or bugfix.
3. Commit your changes with clear messages.
4. Submit a pull request.

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.

---

Feel free to open an issue if you encounter any problems or have suggestions for new features!
