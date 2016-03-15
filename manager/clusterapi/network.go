package clusterapi

import (
	"net"

	"github.com/docker/swarm-v2/api"
	"github.com/docker/swarm-v2/identity"
	"github.com/docker/swarm-v2/manager/state"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func validateDriver(driver *api.Driver) error {
	if driver == nil {
		// It is ok to not specify the driver. We will choose
		// a default driver.
		return nil
	}

	if driver.Name == "" {
		return grpc.Errorf(codes.InvalidArgument, "driver name: if driver is specified name is required")
	}

	return nil
}

func validateIPAMConfiguration(ipamConf *api.IPAMConfiguration) error {
	if ipamConf == nil {
		return grpc.Errorf(codes.InvalidArgument, "ipam configuration: cannot be empty")
	}

	_, subnet, err := net.ParseCIDR(ipamConf.Subnet)
	if err != nil {
		return grpc.Errorf(codes.InvalidArgument, "ipam configuration: invalid subnet %s", ipamConf.Subnet)
	}

	if ipamConf.Range != "" {
		ip, _, err := net.ParseCIDR(ipamConf.Range)
		if err != nil {
			return grpc.Errorf(codes.InvalidArgument, "ipam configuration: invalid range %s", ipamConf.Range)
		}

		if !subnet.Contains(ip) {
			return grpc.Errorf(codes.InvalidArgument, "ipam configuration: subnet %s does not contain range %s", ipamConf.Subnet, ipamConf.Range)
		}
	}

	if ipamConf.Gateway != "" {
		ip := net.ParseIP(ipamConf.Gateway)
		if ip == nil {
			return grpc.Errorf(codes.InvalidArgument, "ipam configuration: invalid gateway %s", ipamConf.Gateway)
		}

		if !subnet.Contains(ip) {
			return grpc.Errorf(codes.InvalidArgument, "ipam configuration: subnet %s does not contain gateway %s", ipamConf.Subnet, ipamConf.Gateway)
		}
	}

	return nil
}

func validateIPAM(ipam *api.IPAMOptions) error {
	if ipam == nil {
		// It is ok to not specify any IPAM configurations. We
		// will choose good defaults.
		return nil
	}

	if err := validateDriver(ipam.Driver); err != nil {
		return err
	}

	for _, ipamConf := range ipam.Configurations {
		if err := validateIPAMConfiguration(ipamConf); err != nil {
			return err
		}
	}

	return nil
}

func validateNetworkSpec(spec *api.NetworkSpec) error {
	if spec == nil {
		return grpc.Errorf(codes.InvalidArgument, errInvalidArgument.Error())
	}

	if err := validateMeta(spec.Meta); err != nil {
		return err
	}

	if err := validateDriver(spec.DriverConfiguration); err != nil {
		return err
	}

	if err := validateIPAM(spec.IPAM); err != nil {
		return err
	}

	return nil
}

// CreateNetwork creates and return a Network based on the provided NetworkSpec.
// - Returns `InvalidArgument` if the NetworkSpec is malformed.
// - Returns an error if the creation fails.
func (s *Server) CreateNetwork(ctx context.Context, request *api.CreateNetworkRequest) (*api.CreateNetworkResponse, error) {
	if err := validateNetworkSpec(request.Spec); err != nil {
		return nil, err
	}

	// TODO(mrjana): Consider using `Name` as a primary key to handle
	// duplicate creations. See #65
	n := &api.Network{
		ID:   identity.NewID(),
		Spec: request.Spec,
	}

	err := s.store.Update(func(tx state.Tx) error {
		return tx.Networks().Create(n)
	})
	if err != nil {
		return nil, err
	}

	return &api.CreateNetworkResponse{
		Network: n,
	}, nil
}

// GetNetwork returns a Network given a NetworkID.
// - Returns `InvalidArgument` if NetworkID is not provided.
// - Returns `NotFound` if the Network is not found.
func (s *Server) GetNetwork(ctx context.Context, request *api.GetNetworkRequest) (*api.GetNetworkResponse, error) {
	if request.NetworkID == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, errInvalidArgument.Error())
	}

	var n *api.Network
	err := s.store.View(func(tx state.ReadTx) error {
		n = tx.Networks().Get(request.NetworkID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, grpc.Errorf(codes.NotFound, "network %s not found", request.NetworkID)
	}
	return &api.GetNetworkResponse{
		Network: n,
	}, nil
}

// RemoveNetwork removes a Network referenced by NetworkID.
// - Returns `InvalidArgument` if NetworkID is not provided.
// - Returns `NotFound` if the Network is not found.
// - Returns an error if the deletion fails.
func (s *Server) RemoveNetwork(ctx context.Context, request *api.RemoveNetworkRequest) (*api.RemoveNetworkResponse, error) {
	if request.NetworkID == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, errInvalidArgument.Error())
	}

	err := s.store.Update(func(tx state.Tx) error {
		return tx.Networks().Delete(request.NetworkID)
	})
	if err != nil {
		if err == state.ErrNotExist {
			return nil, grpc.Errorf(codes.NotFound, "network %s not found", request.NetworkID)
		}
		return nil, err
	}
	return &api.RemoveNetworkResponse{}, nil
}

// ListNetworks returns a list of all networks.
func (s *Server) ListNetworks(ctx context.Context, request *api.ListNetworksRequest) (*api.ListNetworksResponse, error) {
	var networks []*api.Network
	err := s.store.View(func(tx state.ReadTx) error {
		var err error

		networks, err = tx.Networks().Find(state.All)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &api.ListNetworksResponse{
		Networks: networks,
	}, nil
}
