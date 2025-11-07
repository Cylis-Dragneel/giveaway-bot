// internal/bot/handlers.go
package bot

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/Cylis-Dragneel/giveaway-bot/internal/db"
	"github.com/Cylis-Dragneel/giveaway-bot/internal/models"
	"github.com/bwmarrin/discordgo"
)

func InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		handleButtonClick(s, i)
	case discordgo.InteractionModalSubmit:
		handleModalSubmit(s, i)
	}
}

func handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	switch data.Name {
	case "create-giveaway":
		createGiveaway(s, i)
	case "list-giveaways":
		listGiveaways(s, i)
	case "my-giveaways":
		myGiveaways(s, i)
	}
}

func handleButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	userID := i.Member.User.ID
	messageID := i.Message.ID

	if customID == "enter_giveaway" {
		handleEnterGiveaway(s, i, userID, messageID)
	} else if strings.HasPrefix(customID, "list_participants_") {
		pageStr := strings.TrimPrefix(customID, "list_participants_")
		page, _ := strconv.Atoi(pageStr)
		showParticipants(s, i, page, messageID)
	} else if strings.HasPrefix(customID, "next_page_") || strings.HasPrefix(customID, "prev_page_") {
		parts := strings.Split(customID, "_")
		page, _ := strconv.Atoi(parts[2])
		if parts[0] == "prev" {
			page--
		} else {
			page++
		}
		showParticipants(s, i, page, messageID)
	} else if customID == "reroll" {
		handleReroll(s, i)
	}
}

func createGiveaway(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	title := options[0].StringValue()
	endStr := options[1].StringValue()
	var roleID string
	if len(options) > 2 {
		roleID = options[2].RoleValue(nil, "").ID
	}

	endTime, err := models.ParseEndTime(endStr)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Invalid end time format: " + err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Creating giveaway...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	embed := models.CreateGiveawayEmbed(title, endTime, roleID, 0)
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji:    &discordgo.ComponentEmoji{Name: "🎉"},
					Style:    discordgo.PrimaryButton,
					CustomID: "enter_giveaway",
				},
				discordgo.Button{
					Label:    "Participants",
					Style:    discordgo.SecondaryButton,
					CustomID: "list_participants_1",
				},
			},
		},
	}

	msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: components,
	})
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: ptr("Error sending message: " + err.Error()),
		})
		return
	}

	ga := &models.Giveaway{
		ID:           msg.ID,
		Title:        title,
		EndTime:      endTime,
		RoleID:       roleID,
		Participants: []string{},
		ChannelID:    i.ChannelID,
		MessageID:    msg.ID,
	}

	duration := time.Until(endTime)
	ga.Timer = time.AfterFunc(duration, func() {
		models.EndGiveaway(GetSession(), ga)
	})

	models.Giveaways[msg.ID] = ga
	db.SaveGiveaway(ga)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: ptr("Giveaway created!"),
	})
}

func handleEnterGiveaway(s *discordgo.Session, i *discordgo.InteractionCreate, userID, messageID string) {
	ga, ok := models.Giveaways[messageID]
	if !ok || time.Now().After(ga.EndTime) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Giveaway not found or has ended.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if ga.RoleID != "" {
		hasRole := false
		for _, role := range i.Member.Roles {
			if role == ga.RoleID {
				hasRole = true
				break
			}
		}
		if !hasRole {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You don't have the required role to join.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	isParticipant := false
	for _, p := range ga.Participants {
		if p == userID {
			isParticipant = true
			break
		}
	}

	if isParticipant {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "leave_giveaway_modal_" + messageID,
				Title:    "Confirm Leave Giveaway",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "leave_confirmation",
								Label:       "Type LEAVE to confirm",
								Style:       discordgo.TextInputShort,
								Placeholder: "LEAVE",
								Required:    true,
							},
						},
					},
				},
			},
		})
	} else {
		ga.Participants = append(ga.Participants, userID)
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You have entered the giveaway!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	log.Printf("Modal CustomID: %s, Components: %+v", data.CustomID, data.Components)
	if strings.HasPrefix(data.CustomID, "leave_giveaway_modal_") {
		messageID := strings.TrimPrefix(data.CustomID, "leave_giveaway_modal_")
		userID := i.Member.User.ID

		// Extract text input from modal
		var input string
		for _, component := range data.Components {
			if actionRow, ok := component.(*discordgo.ActionsRow); ok {
				for _, comp := range actionRow.Components {
					if textInput, ok := comp.(*discordgo.TextInput); ok && textInput.CustomID == "leave_confirmation" {
						input = textInput.Value
						break
					}
				}
			}
		}

		if input != "LEAVE" {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Invalid input. You must type 'LEAVE' exactly.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println("Error responding to modal submission:", err)
			}
			return
		}

		ga, ok := models.Giveaways[messageID]
		if !ok {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Giveaway not found.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Println("Error responding to modal submission:", err)
			}
			return
		}

		for idx, p := range ga.Participants {
			if p == userID {
				ga.Participants = append(ga.Participants[:idx], ga.Participants[idx+1:]...)
				break
			}
		}
		models.UpdateGiveawayEmbed(s, ga)
		db.SaveParticipants(ga.ID, ga.Participants)

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You have left the giveaway.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Println("Error responding to modal submission:", err)
		}
	}
}

func handleReroll(s *discordgo.Session, i *discordgo.InteractionCreate) {
	originalMsgID := i.Message.ID
	ga, ok := models.Giveaways[originalMsgID]
	var participants []string
	if ok {
		participants = ga.Participants
	} else {
		participants = db.LoadParticipants(originalMsgID)
	}

	if len(participants) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No participants to reroll.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	winnerIdx := rand.Intn(len(participants))
	winnerID := participants[winnerIdx]

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("<@%s>", winnerID),
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Giveaway Rerolled!",
					Description: fmt.Sprintf("New Winner: <@%s>", winnerID),
					Color:       0xff0000,
				},
			},
		},
	})
}

func showParticipants(s *discordgo.Session, i *discordgo.InteractionCreate, page int, messageID string) {
	ga, ok := models.Giveaways[messageID]
	var participants []string
	if ok {
		participants = ga.Participants
	} else {
		participants = db.LoadParticipants(messageID)
	}

	if len(participants) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No participants.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	perPage := 10
	start := (page - 1) * perPage
	end := start + perPage
	if end > len(participants) {
		end = len(participants)
	}
	list := ""
	for _, p := range participants[start:end] {
		list += fmt.Sprintf("<@%s>\n", p)
	}

	totalPages := (len(participants) + perPage - 1) / perPage

	components := []discordgo.MessageComponent{}
	if totalPages > 1 {
		row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{}}
		if page > 1 {
			row.Components = append(row.Components, discordgo.Button{
				Label:    "Previous",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("prev_page_%d", page),
			})
		}
		if page < totalPages {
			row.Components = append(row.Components, discordgo.Button{
				Label:    "Next",
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("next_page_%d", page),
			})
		}
		components = append(components, row)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    fmt.Sprintf("Participants (Page %d/%d):\n%s", page, totalPages, list),
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
}

func listGiveaways(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if len(models.Giveaways) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No running giveaways.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	list := ""
	for _, ga := range models.Giveaways {
		list += fmt.Sprintf("- %s (Ends: %s)\n", ga.Title, ga.EndTime.Format(time.RFC1123))
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Running Giveaways:\n" + list,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func myGiveaways(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := i.Member.User.ID
	list := ""
	for _, ga := range models.Giveaways {
		for _, p := range ga.Participants {
			if p == userID {
				list += fmt.Sprintf("- %s (Ends: %s)\n", ga.Title, ga.EndTime.Format(time.RFC1123))
				break
			}
		}
	}

	if list == "" {
		list = "You haven't entered any giveaways."
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Your Entered Giveaways:\n" + list,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func ptr(s string) *string {
	return &s
}
