package api

import (
	"net/http"
	"nagomi-core/internal/api/middleware"
	"nagomi-core/internal/gen/nagomi/v1/nagomiv1connect"
	"nagomi-core/internal/service"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"github.com/charmbracelet/log"
)

type Server struct {
	services    *service.Services
	log         *log.Logger
	healthCheck grpchealth.Checker
}

func NewServer(services *service.Services, logger *log.Logger) *Server {
	healthCheck := grpchealth.NewStaticChecker(
		"nagomi.v1.UserService",
		"nagomi.v1.AccountService",
		"nagomi.v1.TransactionService",
		"nagomi.v1.CategoryService",
		"nagomi.v1.RuleService",
		"nagomi.v1.DashboardService",
		"nagomi.v1.ReceiptService",
		"nagomi.v1.ConnectorService",
		"nagomi.v1.ConnectionsService",
		"nagomi.v1.StatementService",
	)

	return &Server{
		services:    services,
		log:         logger,
		healthCheck: healthCheck,
	}
}

func (s *Server) SetServingStatus(service string, healthy bool) {
	if checker, ok := s.healthCheck.(*grpchealth.StaticChecker); ok {
		if healthy {
			checker.SetStatus(service, grpchealth.StatusServing)
		} else {
			checker.SetStatus(service, grpchealth.StatusNotServing)
		}
	}
}

func (s *Server) GetHandler(authConfig *middleware.AuthConfig) http.Handler {
	if authConfig == nil {
		s.log.Fatal("auth configuration is required")
	}

	mux := http.NewServeMux()
	s.registerServices(mux)

	stack := middleware.CreateStack(
		middleware.CORS(),
		middleware.Auth(authConfig, s.log),
		middleware.UserContext(),
	)

	return stack(mux)
}

func (s *Server) registerServices(mux *http.ServeMux) {
	healthPath, healthHandler := grpchealth.NewHandler(s.healthCheck)
	mux.Handle(healthPath, healthHandler)

	reflector := grpcreflect.NewStaticReflector(
		"nagomi.v1.UserService",
		"nagomi.v1.AccountService",
		"nagomi.v1.TransactionService",
		"nagomi.v1.CategoryService",
		"nagomi.v1.RuleService",
		"nagomi.v1.DashboardService",
		"nagomi.v1.ReceiptService",
		"nagomi.v1.ConnectorService",
		"nagomi.v1.ConnectionsService",
		"nagomi.v1.StatementService",
	)
	reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(reflectPath, reflectHandler)
	reflectPathAlpha, reflectHandlerAlpha := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(reflectPathAlpha, reflectHandlerAlpha)

	interceptors := connect.WithInterceptors(
		middleware.ConnectLoggingInterceptor(s.log),
		middleware.EnsureUserInterceptor(s.services.Users, s.log),
		middleware.UserIDExtractor(),
	)

	path, handler := nagomiv1connect.NewUserServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewAccountServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewTransactionServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewCategoryServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewRuleServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewDashboardServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewReceiptServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewConnectorServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewConnectionsServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	path, handler = nagomiv1connect.NewStatementServiceHandler(s, interceptors)
	mux.Handle(path, handler)

	s.log.Info("all connect-go services registered",
		"health_endpoint", healthPath,
	)
}
