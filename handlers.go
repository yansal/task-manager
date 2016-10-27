package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/gin-gonic/gin.v1"
)

func init() {
	router = gin.Default()
	router.GET("/tasks/", getTasksHandler)
	router.GET("/tasks/:id", getTasksIDHandler)
	router.GET("/users/:id/tasks", getUsersIDTasksHandler)
	router.GET("/tasks/:id/comments", getTasksIDCommentsHandler)
	router.GET("/users/:id/comments", getUsersIDCommentsHandler)
	authorized := router.Group("/", authMiddleware)
	authorized.POST("/tasks/", postTasksHandler)
	authorized.PATCH("/tasks/:id", patchTasksIDHandler)
	authorized.POST("/tasks/:id/comments", postTasksIDCommentsHandler)
}

func getTasksHandler(c *gin.Context) {
	var tasks []TaskResource
	if err := selectTasks.Select(&tasks); err != nil {
		log.Printf("couldn't select from tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func getTasksIDHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var task TaskResource
	err = selectTaskWhereID.Get(&task, id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	if err != nil {
		log.Printf("couldn't select from tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, task)
}

func postTasksHandler(c *gin.Context) {
	user, exists := c.Get(gin.AuthUserKey)
	if !exists {
		log.Print("No user in POST /tasks/ handler context")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	var task = Task{UserID: user.(User).ID}
	if err := c.BindJSON(&task); err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	if _, err := insertTask.Exec(task.Name, task.UserID, task.Description, task.Progression); err != nil {
		log.Printf("couldn't insert to tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusCreated, "", nil)
}

// Patches is a patch document according to https://tools.ietf.org/html/rfc6902
type Patches []struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func patchTasksIDHandler(c *gin.Context) {
	user, exists := c.Get(gin.AuthUserKey)
	if !exists {
		log.Print("No user in PATCH /tasks/:id handler context")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var task TaskResource
	err = selectTaskWhereID.Get(&task, taskID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, nil)
		return
	}
	if err != nil {
		log.Printf("couldn't select from tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if task.User.ID != user.(User).ID {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// TODO: Validate patch document
	var patches Patches
	if err = c.BindJSON(&patches); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	// TODO: Apply patch document
	tx, err := db.Beginx()
	if err != nil {
		log.Printf("couldn't begin: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	for _, patch := range patches {
		if patch.Op != "replace" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf(`op must be "replace"; got %v`, patch.Op))
			return
		}
		if patch.Path != "/name" {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf(`path must be "/name"; got %v`, patch.Op))
			return
		}
		switch t := patch.Value.(type) {
		case string:
		default:
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf(`value must be a string; got %T`, t))
			return
		}

		if _, err := tx.Stmtx(updateTasksName).Exec(patch.Value, taskID); err != nil {
			log.Printf("couldn't update tasks: %v", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("couldn't commit: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusNoContent, "", nil)
}

func getUsersIDTasksHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var tasks []TaskResource
	if err = selectTasksWhereUserID.Select(&tasks, id); err != nil {
		log.Printf("couldn't select from tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func postTasksIDCommentsHandler(c *gin.Context) {
	user, exists := c.Get(gin.AuthUserKey)
	if !exists {
		log.Print("No user in POST /tasks/:id/comments handler context")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var task TaskResource
	err = selectTaskWhereID.Get(&task, taskID)
	if err == sql.ErrNoRows {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("couldn't select from tasks: %v:", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	var comment = Comment{UserID: user.(User).ID, TaskID: taskID}
	if err := c.BindJSON(&comment); err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	if _, err := insertComment.Exec(comment.UserID, comment.TaskID, comment.Content); err != nil {
		log.Printf("couldn't insert to comments: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusCreated, "", nil)
}

func getTasksIDCommentsHandler(c *gin.Context) {
	taskID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var comments []CommentResource
	if err = selectCommentsWhereTaskID.Select(&comments, taskID); err != nil {
		log.Printf("couldn't select from comments: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, comments)
}

func getUsersIDCommentsHandler(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, err)
		return
	}

	var comments []CommentResource
	if err := selectCommentsWhereUserID.Select(&comments, userID); err != nil {
		log.Printf("couldn't select from tasks: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, comments)
}

func authMiddleware(c *gin.Context) {
	header := c.Request.Header.Get("Authorization")
	fields := strings.Fields(header)
	if len(fields) != 2 || fields[0] != "Token" {
		c.Header("WWW-Authenticate", "Token")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	var user User
	err := selectUsersWhereToken.Get(&user, fields[1])
	if err == sql.ErrNoRows {
		c.Header("WWW-Authenticate", "Token")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("couldn't select from users: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Set(gin.AuthUserKey, user)
}
