package caldav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/icalendar"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

const (
	defaultDatabaseName = "openplanner.db"
	defaultAddr         = "127.0.0.1:8080"

	davNamespace    = "DAV:"
	caldavNamespace = "urn:ietf:params:xml:ns:caldav"
)

type Options struct {
	Addr         string
	DatabasePath string
}

type Server struct {
	service *service.Service
}

type runtime struct {
	server *Server
	close  func() error
}

type targetKind int

const (
	targetUnknown targetKind = iota
	targetRoot
	targetPrincipal
	targetCalendarHome
	targetCalendarCollection
	targetCalendarObject
)

type target struct {
	kind       targetKind
	href       string
	calendarID string
	resource   string
}

type propertyValue struct {
	name       xml.Name
	text       string
	href       string
	resource   []xml.Name
	components []string
}

type response struct {
	href     string
	ok       []propertyValue
	notFound []xml.Name
}

type calendarQuery struct {
	includeEvents bool
	includeTasks  bool
	hasTimeRange  bool
	from          time.Time
	to            time.Time
}

func NewServer(service *service.Service) *Server {
	return &Server{service: service}
}

func ListenAndServe(ctx context.Context, options Options) error {
	if options.Addr == "" {
		options.Addr = defaultAddr
	}
	runtime, err := Open(options)
	if err != nil {
		return err
	}
	defer func() {
		_ = runtime.Close()
	}()

	listener, err := net.Listen("tcp", options.Addr)
	if err != nil {
		return fmt.Errorf("listen caldav: %w", err)
	}

	httpServer := &http.Server{Handler: runtime.server}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		}()
	}

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve caldav: %w", err)
	}
	return nil
}

func Open(options Options) (*runtime, error) {
	databasePath, err := resolveDatabasePath(options.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	repository, err := store.Open(databasePath)
	if err != nil {
		return nil, err
	}
	return &runtime{
		server: NewServer(service.New(repository)),
		close:  repository.Close,
	}, nil
}

func (runtime *runtime) Close() error {
	if runtime == nil || runtime.close == nil {
		return nil
	}
	return runtime.close()
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/.well-known/caldav" {
		http.Redirect(writer, request, "/caldav/", http.StatusMovedPermanently)
		return
	}

	writer.Header().Set("DAV", "1, calendar-access")
	writer.Header().Set("MS-Author-Via", "DAV")

	target, ok := route(request.URL.Path)
	if !ok {
		http.NotFound(writer, request)
		return
	}

	switch request.Method {
	case http.MethodOptions:
		writer.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		writer.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		server.handlePropfind(writer, request, target)
	case "REPORT":
		server.handleReport(writer, request, target)
	case http.MethodGet, http.MethodHead:
		server.handleGet(writer, request, target)
	case http.MethodPut:
		server.handlePut(writer, request, target)
	case http.MethodDelete:
		server.handleDelete(writer, request, target)
	default:
		writer.Header().Set("Allow", "OPTIONS, PROPFIND, REPORT, GET, HEAD, PUT, DELETE")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (server *Server) handlePropfind(writer http.ResponseWriter, request *http.Request, routeTarget target) {
	if routeTarget.kind == targetUnknown {
		http.NotFound(writer, request)
		return
	}

	allProperties, requested, err := parsePropfind(request.Body)
	if err != nil {
		http.Error(writer, "invalid PROPFIND body", http.StatusBadRequest)
		return
	}

	depth := request.Header.Get("Depth")
	responses := []response{}
	switch routeTarget.kind {
	case targetRoot, targetPrincipal:
		responses = append(responses, server.responseForTarget(routeTarget, allProperties, requested))
	case targetCalendarHome:
		responses = append(responses, server.responseForTarget(routeTarget, allProperties, requested))
		if depth == "1" {
			calendars, err := server.listCalendars()
			if err != nil {
				writeServiceError(writer, err)
				return
			}
			for _, calendar := range calendars {
				child := target{
					kind:       targetCalendarCollection,
					href:       calendarHref(calendar.ID),
					calendarID: calendar.ID,
				}
				responses = append(responses, server.responseForTarget(child, allProperties, requested))
			}
		}
	case targetCalendarCollection:
		if _, err := server.service.GetCalendar(routeTarget.calendarID); err != nil {
			writeServiceError(writer, err)
			return
		}
		responses = append(responses, server.responseForTarget(routeTarget, allProperties, requested))
		if depth == "1" {
			objects, err := server.service.ListICalendarObjects(routeTarget.calendarID)
			if err != nil {
				writeServiceError(writer, err)
				return
			}
			for _, object := range objects {
				child := target{
					kind:       targetCalendarObject,
					href:       objectHref(routeTarget.calendarID, object.ResourceName()),
					calendarID: routeTarget.calendarID,
					resource:   object.ResourceName(),
				}
				responses = append(responses, server.responseForTarget(child, allProperties, requested))
			}
		}
	case targetCalendarObject:
		if _, err := server.service.ResolveICalendarObject(routeTarget.calendarID, routeTarget.resource); err != nil {
			writeServiceError(writer, err)
			return
		}
		responses = append(responses, server.responseForTarget(routeTarget, allProperties, requested))
	}

	writeMultiStatus(writer, responses)
}

func (server *Server) handleReport(writer http.ResponseWriter, request *http.Request, routeTarget target) {
	if routeTarget.kind != targetCalendarCollection {
		http.Error(writer, "REPORT is only supported for calendar collections", http.StatusMethodNotAllowed)
		return
	}
	query, err := parseCalendarQuery(request.Body)
	if err != nil {
		http.Error(writer, "invalid calendar-query REPORT", http.StatusBadRequest)
		return
	}

	objects, err := server.queryObjects(routeTarget.calendarID, query)
	if err != nil {
		writeServiceError(writer, err)
		return
	}

	responses := make([]response, 0, len(objects))
	for _, object := range objects {
		export, err := server.service.ExportICalendarObject(routeTarget.calendarID, object.ResourceName())
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		responses = append(responses, response{
			href: objectHref(routeTarget.calendarID, object.ResourceName()),
			ok: []propertyValue{
				{name: davName("getetag"), text: etag(export.Content)},
				{name: caldavName("calendar-data"), text: export.Content},
			},
		})
	}
	writeMultiStatus(writer, responses)
}

func (server *Server) handleGet(writer http.ResponseWriter, request *http.Request, target target) {
	if target.kind != targetCalendarObject {
		http.Error(writer, "GET is only supported for calendar objects", http.StatusMethodNotAllowed)
		return
	}

	export, err := server.service.ExportICalendarObject(target.calendarID, target.resource)
	if err != nil {
		writeServiceError(writer, err)
		return
	}

	writer.Header().Set("Content-Type", export.ContentType)
	writer.Header().Set("ETag", etag(export.Content))
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(export.Content)))
	if request.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(writer, export.Content)
}

func (server *Server) handlePut(writer http.ResponseWriter, request *http.Request, target target) {
	if target.kind != targetCalendarObject {
		http.Error(writer, "PUT is only supported for calendar objects", http.StatusMethodNotAllowed)
		return
	}
	if contentType := request.Header.Get("Content-Type"); contentType != "" && !strings.Contains(strings.ToLower(contentType), "text/calendar") {
		http.Error(writer, "PUT requires text/calendar content", http.StatusUnsupportedMediaType)
		return
	}

	content, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
	if err != nil {
		http.Error(writer, "read request body", http.StatusBadRequest)
		return
	}
	if err := validateSingleCalendarObject(string(content)); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}

	imported, err := server.service.ImportICalendar(domain.ICalendarImportRequest{
		CalendarID: target.calendarID,
		Content:    string(content),
	})
	if err != nil {
		writeServiceError(writer, err)
		return
	}

	status := http.StatusNoContent
	for _, write := range imported.Writes {
		if (write.Kind == "event" || write.Kind == "task") && write.Status == "created" {
			status = http.StatusCreated
			if write.ID != "" {
				writer.Header().Set("Location", objectHref(target.calendarID, write.ID+".ics"))
			}
			break
		}
	}
	writer.WriteHeader(status)
}

func (server *Server) handleDelete(writer http.ResponseWriter, _ *http.Request, target target) {
	if target.kind != targetCalendarObject {
		http.Error(writer, "DELETE is only supported for calendar objects", http.StatusMethodNotAllowed)
		return
	}
	if err := server.service.DeleteICalendarObject(target.calendarID, target.resource); err != nil {
		writeServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) responseForTarget(target target, allProperties bool, requested []xml.Name) response {
	supported, err := server.propertiesForTarget(target)
	if err != nil {
		return response{
			href:     target.href,
			notFound: requested,
		}
	}

	if allProperties {
		return response{href: target.href, ok: supported}
	}

	okProps := []propertyValue{}
	notFound := []xml.Name{}
	for _, requestedName := range requested {
		if value, ok := findProperty(supported, requestedName); ok {
			okProps = append(okProps, value)
			continue
		}
		notFound = append(notFound, requestedName)
	}
	return response{href: target.href, ok: okProps, notFound: notFound}
}

func (server *Server) propertiesForTarget(target target) ([]propertyValue, error) {
	switch target.kind {
	case targetRoot:
		return []propertyValue{
			{name: davName("resourcetype"), resource: []xml.Name{davName("collection")}},
			{name: davName("displayname"), text: "OpenPlanner CalDAV"},
			{name: davName("current-user-principal"), href: "/caldav/principals/local/"},
			{name: caldavName("calendar-home-set"), href: "/caldav/calendars/local/"},
		}, nil
	case targetPrincipal:
		return []propertyValue{
			{name: davName("resourcetype"), resource: []xml.Name{davName("collection"), davName("principal")}},
			{name: davName("displayname"), text: "local"},
			{name: caldavName("calendar-home-set"), href: "/caldav/calendars/local/"},
		}, nil
	case targetCalendarHome:
		return []propertyValue{
			{name: davName("resourcetype"), resource: []xml.Name{davName("collection")}},
			{name: davName("displayname"), text: "OpenPlanner Calendars"},
		}, nil
	case targetCalendarCollection:
		calendar, err := server.service.GetCalendar(target.calendarID)
		if err != nil {
			return nil, err
		}
		return []propertyValue{
			{name: davName("resourcetype"), resource: []xml.Name{davName("collection"), caldavName("calendar")}},
			{name: davName("displayname"), text: calendar.Name},
			{name: caldavName("supported-calendar-component-set"), components: []string{"VEVENT", "VTODO"}},
		}, nil
	case targetCalendarObject:
		export, err := server.service.ExportICalendarObject(target.calendarID, target.resource)
		if err != nil {
			return nil, err
		}
		return []propertyValue{
			{name: davName("getcontenttype"), text: export.ContentType},
			{name: davName("getcontentlength"), text: fmt.Sprintf("%d", len(export.Content))},
			{name: davName("getetag"), text: etag(export.Content)},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported CalDAV target")
	}
}

func (server *Server) queryObjects(calendarID string, query calendarQuery) ([]service.ICalendarObject, error) {
	objects, err := server.service.ListICalendarObjects(calendarID)
	if err != nil {
		return nil, err
	}
	if !query.includeEvents && !query.includeTasks {
		return objects, nil
	}

	includeIDs := map[string]bool{}
	if query.includeEvents && query.hasTimeRange {
		cursor := ""
		for {
			page, err := server.service.ListAgenda(domain.AgendaParams{
				From:   query.from,
				To:     query.to,
				Cursor: cursor,
				Limit:  200,
			})
			if err != nil {
				return nil, err
			}
			for _, item := range page.Items {
				if item.Kind == domain.AgendaItemKindEvent && item.CalendarID == calendarID {
					includeIDs[item.SourceID] = true
				}
			}
			if page.NextCursor == nil {
				break
			}
			cursor = *page.NextCursor
		}
	}

	filtered := make([]service.ICalendarObject, 0, len(objects))
	for _, object := range objects {
		switch object.Kind {
		case service.ICalendarObjectKindEvent:
			if query.includeEvents && (!query.hasTimeRange || includeIDs[object.ID()]) {
				filtered = append(filtered, object)
			}
		case service.ICalendarObjectKindTask:
			if query.includeTasks {
				filtered = append(filtered, object)
			}
		}
	}
	return filtered, nil
}

func parsePropfind(reader io.Reader) (bool, []xml.Name, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return false, nil, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return true, nil, nil
	}

	decoder := xml.NewDecoder(bytes.NewReader(content))
	inProp := false
	requested := []xml.Name{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch {
			case value.Name.Local == "allprop" || value.Name.Local == "propname":
				return true, nil, nil
			case value.Name.Local == "prop":
				inProp = true
			case inProp:
				requested = append(requested, value.Name)
			}
		case xml.EndElement:
			if value.Name.Local == "prop" {
				inProp = false
			}
		}
	}
	if len(requested) == 0 {
		return true, nil, nil
	}
	return false, requested, nil
}

func parseCalendarQuery(reader io.Reader) (calendarQuery, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return calendarQuery{}, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return calendarQuery{}, nil
	}

	query := calendarQuery{}
	decoder := xml.NewDecoder(bytes.NewReader(content))
	componentStack := []string{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return calendarQuery{}, err
		}

		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "comp-filter" {
				componentName := strings.ToUpper(attr(value, "name"))
				componentStack = append(componentStack, componentName)
				switch componentName {
				case "VEVENT":
					query.includeEvents = true
				case "VTODO":
					query.includeTasks = true
				}
				continue
			}
			if value.Name.Local == "time-range" && slices.Contains(componentStack, "VEVENT") {
				from, to, err := parseTimeRange(value)
				if err != nil {
					return calendarQuery{}, err
				}
				query.hasTimeRange = true
				query.from = from
				query.to = to
			}
		case xml.EndElement:
			if value.Name.Local == "comp-filter" && len(componentStack) > 0 {
				componentStack = componentStack[:len(componentStack)-1]
			}
		}
	}
	return query, nil
}

func parseTimeRange(element xml.StartElement) (time.Time, time.Time, error) {
	start := attr(element, "start")
	end := attr(element, "end")
	from := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	var err error
	if start != "" {
		from, err = time.Parse("20060102T150405Z", start)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if end != "" {
		to, err = time.Parse("20060102T150405Z", end)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("time-range end must be after start")
	}
	return from, to, nil
}

func validateSingleCalendarObject(content string) error {
	parsed, err := icalendar.ParseImport(content)
	if err != nil {
		return err
	}
	count := len(parsed.Events) + len(parsed.Tasks)
	if count != 1 || len(parsed.EventChanges) > 0 || len(parsed.TaskChanges) > 0 {
		return fmt.Errorf("PUT requires exactly one base VEVENT or VTODO")
	}
	if len(parsed.Skips) > 0 {
		return fmt.Errorf("PUT calendar object was skipped: %s", parsed.Skips[0].Reason)
	}
	return nil
}

func writeMultiStatus(writer http.ResponseWriter, responses []response) {
	writer.Header().Set("Content-Type", `application/xml; charset=utf-8`)
	writer.WriteHeader(207)

	encoder := xml.NewEncoder(writer)
	_ = encoder.EncodeToken(xml.StartElement{Name: davName("multistatus")})
	for _, response := range responses {
		writeResponse(encoder, response)
	}
	_ = encoder.EncodeToken(xml.EndElement{Name: davName("multistatus")})
	_ = encoder.Flush()
}

func writeResponse(encoder *xml.Encoder, response response) {
	start := xml.StartElement{Name: davName("response")}
	_ = encoder.EncodeToken(start)
	writeTextElement(encoder, davName("href"), response.href)
	if len(response.ok) > 0 {
		writePropstat(encoder, response.ok, nil, "HTTP/1.1 200 OK")
	}
	if len(response.notFound) > 0 {
		writePropstat(encoder, nil, response.notFound, "HTTP/1.1 404 Not Found")
	}
	_ = encoder.EncodeToken(start.End())
}

func writePropstat(encoder *xml.Encoder, values []propertyValue, names []xml.Name, status string) {
	start := xml.StartElement{Name: davName("propstat")}
	_ = encoder.EncodeToken(start)

	propStart := xml.StartElement{Name: davName("prop")}
	_ = encoder.EncodeToken(propStart)
	for _, value := range values {
		writeProperty(encoder, value)
	}
	for _, name := range names {
		_ = encoder.EncodeToken(xml.StartElement{Name: name})
		_ = encoder.EncodeToken(xml.EndElement{Name: name})
	}
	_ = encoder.EncodeToken(propStart.End())
	writeTextElement(encoder, davName("status"), status)
	_ = encoder.EncodeToken(start.End())
}

func writeProperty(encoder *xml.Encoder, value propertyValue) {
	start := xml.StartElement{Name: value.name}
	_ = encoder.EncodeToken(start)
	switch {
	case value.href != "":
		writeTextElement(encoder, davName("href"), value.href)
	case len(value.resource) > 0:
		for _, name := range value.resource {
			_ = encoder.EncodeToken(xml.StartElement{Name: name})
			_ = encoder.EncodeToken(xml.EndElement{Name: name})
		}
	case len(value.components) > 0:
		for _, component := range value.components {
			element := xml.StartElement{
				Name: caldavName("comp"),
				Attr: []xml.Attr{{Name: xml.Name{Local: "name"}, Value: component}},
			}
			_ = encoder.EncodeToken(element)
			_ = encoder.EncodeToken(element.End())
		}
	default:
		_ = encoder.EncodeToken(xml.CharData([]byte(value.text)))
	}
	_ = encoder.EncodeToken(start.End())
}

func writeTextElement(encoder *xml.Encoder, name xml.Name, text string) {
	start := xml.StartElement{Name: name}
	_ = encoder.EncodeToken(start)
	_ = encoder.EncodeToken(xml.CharData([]byte(text)))
	_ = encoder.EncodeToken(start.End())
}

func writeServiceError(writer http.ResponseWriter, err error) {
	var validationErr *service.ValidationError
	var notFoundErr *service.NotFoundError
	var conflictErr *service.ConflictError
	switch {
	case errors.As(err, &validationErr):
		http.Error(writer, validationErr.Error(), http.StatusBadRequest)
	case errors.As(err, &notFoundErr):
		http.Error(writer, notFoundErr.Error(), http.StatusNotFound)
	case errors.As(err, &conflictErr):
		http.Error(writer, conflictErr.Error(), http.StatusConflict)
	default:
		http.Error(writer, "internal server error", http.StatusInternalServerError)
	}
}

func route(rawPath string) (target, bool) {
	switch rawPath {
	case "/caldav", "/caldav/":
		return target{kind: targetRoot, href: "/caldav/"}, true
	case "/caldav/principals/local", "/caldav/principals/local/":
		return target{kind: targetPrincipal, href: "/caldav/principals/local/"}, true
	case "/caldav/calendars/local", "/caldav/calendars/local/":
		return target{kind: targetCalendarHome, href: "/caldav/calendars/local/"}, true
	}

	const prefix = "/caldav/calendars/local/"
	if !strings.HasPrefix(rawPath, prefix) {
		return target{}, false
	}
	rest := strings.TrimPrefix(rawPath, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return target{kind: targetCalendarHome, href: "/caldav/calendars/local/"}, true
	}

	parts := strings.Split(rest, "/")
	calendarID, err := url.PathUnescape(parts[0])
	if err != nil || calendarID == "" {
		return target{}, false
	}
	if len(parts) == 1 {
		return target{
			kind:       targetCalendarCollection,
			href:       calendarHref(calendarID),
			calendarID: calendarID,
		}, true
	}
	if len(parts) == 2 && strings.HasSuffix(parts[1], ".ics") {
		resource, err := url.PathUnescape(parts[1])
		if err != nil || resource == "" {
			return target{}, false
		}
		return target{
			kind:       targetCalendarObject,
			href:       objectHref(calendarID, resource),
			calendarID: calendarID,
			resource:   resource,
		}, true
	}
	return target{}, false
}

func (server *Server) listCalendars() ([]domain.Calendar, error) {
	calendars := []domain.Calendar{}
	cursor := ""
	for {
		page, err := server.service.ListCalendars(domain.PageParams{Cursor: cursor, Limit: 200})
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, page.Items...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	return calendars, nil
}

func findProperty(values []propertyValue, name xml.Name) (propertyValue, bool) {
	for _, value := range values {
		if sameName(value.name, name) {
			return value, true
		}
	}
	return propertyValue{}, false
}

func sameName(left xml.Name, right xml.Name) bool {
	return left.Local == right.Local && (left.Space == right.Space || left.Space == "" || right.Space == "")
}

func attr(element xml.StartElement, name string) string {
	for _, attr := range element.Attr {
		if strings.EqualFold(attr.Name.Local, name) {
			return strings.TrimSpace(attr.Value)
		}
	}
	return ""
}

func etag(content string) string {
	sum := sha256.Sum256([]byte(content))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func davName(local string) xml.Name {
	return xml.Name{Space: davNamespace, Local: local}
}

func caldavName(local string) xml.Name {
	return xml.Name{Space: caldavNamespace, Local: local}
}

func calendarHref(calendarID string) string {
	return "/caldav/calendars/local/" + pathEscape(calendarID) + "/"
}

func objectHref(calendarID string, resource string) string {
	return calendarHref(calendarID) + pathEscape(resource)
}

func pathEscape(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), "+", "%20")
}

func resolveDatabasePath(databasePath string) (string, error) {
	if databasePath != "" {
		return databasePath, nil
	}
	dataDir, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, defaultDatabaseName), nil
}

func defaultDataDir() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "openplanner"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("resolve user home: empty value")
	}
	return filepath.Join(home, ".local", "share", "openplanner"), nil
}
