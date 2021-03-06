package vbmh

import (
	metal3 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	mockClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	airshipv1 "sipcluster/pkg/api/v1"
	"sipcluster/testutil"
)

const (
	// numNodes is the number of test vBMH objects (nodes) created for each test
	numNodes = 7
)

var _ = Describe("MachineList", func() {
	var machineList *MachineList
	var err error
	BeforeEach(func() {
		nodes := map[string]*Machine{}
		for n := 0; n < numNodes; n++ {
			bmh, _ := testutil.CreateBMH(n, "default", airshipv1.VMControlPlane, 6)
			nodes[bmh.Name], err = NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
			Expect(err).To(BeNil())
		}

		machineList = &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: nodes,
			Log:      ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		err := metal3.AddToScheme(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should report if it has a machine registered for a BMH object", func() {
		for _, bmh := range machineList.Machines {
			Expect(machineList.hasMachine(bmh.BMH)).Should(BeTrue())
		}

		unregisteredMachine := machineList.Machines["node01"]
		unregisteredMachine.BMH.Name = "foo"
		Expect(machineList.hasMachine(unregisteredMachine.BMH)).Should(BeFalse())
	})

	It("Should produce a list of unscheduled BMH objects", func() {
		// "Schedule" two nodes
		machineList.Machines["node00"].BMH.Labels[SipScheduleLabel] = "true"
		machineList.Machines["node01"].BMH.Labels[SipScheduleLabel] = "true"
		scheduledNodes := []metal3.BareMetalHost{
			machineList.Machines["node00"].BMH,
			machineList.Machines["node01"].BMH,
		}

		var objs []runtime.Object
		for _, machine := range machineList.Machines {
			objs = append(objs, &machine.BMH)
		}

		k8sClient := mockClient.NewFakeClient(objs...)
		bmhList, err := machineList.getBMHs(k8sClient)
		Expect(err).To(BeNil())

		// Validate that the BMH list does not contain scheduled nodes
		for _, bmh := range bmhList.Items {
			for _, scheduled := range scheduledNodes {
				Expect(bmh).ToNot(Equal(scheduled))
				Expect(bmh.Labels[SipScheduleLabel]).To(Equal("false"))
			}
		}
	})

	It("Should not produce a list of BMH objects when there are none available for scheduling", func() {
		// "Schedule" all nodes
		var objs []runtime.Object
		for _, machine := range machineList.Machines {
			machine.BMH.Labels[SipScheduleLabel] = "true"
			objs = append(objs, &machine.BMH)
		}

		k8sClient := mockClient.NewFakeClient(objs...)
		_, err := machineList.getBMHs(k8sClient)
		Expect(err).ToNot(BeNil())
	})

	It("Should retrieve the BMH IP from the BMH's NetworkData secret when infra services are defined", func() {
		// Create a BMH with a NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", airshipv1.VMControlPlane, 6)

		// Create BMH and NetworkData secret
		var objsToApply []runtime.Object
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		// Create BMC credential secret
		username := "root"
		password := "test"
		bmcSecret := testutil.CreateBMCAuthSecret(bmh.Name, bmh.Namespace, username, password)

		bmh.Spec.BMC.CredentialsName = bmcSecret.Name
		objsToApply = append(objsToApply, bmcSecret)

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateServiceAddresses(*sipCluster, k8sClient)).To(BeNil())

		Expect(ml.Machines[bmh.Name].Data.IPOnInterface).To(Equal(map[string]string{"oam-ipv4": "32.68.51.139"}))
	})

	It("Should not retrieve the BMH IP from the BMH's NetworkData secret if no infraServices are defined", func() {
		// Create a BMH with a NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", airshipv1.VMControlPlane, 6)

		// Create BMH and NetworkData secret
		var objsToApply []runtime.Object
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		// Create BMC credential secret
		username := "root"
		password := "test"
		bmcSecret := testutil.CreateBMCAuthSecret(bmh.Name, bmh.Namespace, username, password)

		bmh.Spec.BMC.CredentialsName = bmcSecret.Name
		objsToApply = append(objsToApply, bmcSecret)

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateBMCAuth(*sipCluster, k8sClient)).To(BeNil())

		Expect(ml.Machines[bmh.Name].Data.BMCUsername).To(Equal(username))
		Expect(ml.Machines[bmh.Name].Data.BMCPassword).To(Equal(password))
	})

	It("Should not process a BMH when its BMC secret is missing", func() {
		var objsToApply []runtime.Object

		// Create BMH and NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", "master", 6)
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		bmh.Spec.BMC.CredentialsName = "foo-does-not-exist"

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			ReadyForScheduleCount: map[airshipv1.VMRole]int{
				airshipv1.VMControlPlane: 1,
				airshipv1.VMWorker:       0,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateBMCAuth(*sipCluster, k8sClient)).ToNot(BeNil())
	})

	It("Should not process a BMH when its BMC secret is incorrectly formatted", func() {
		var objsToApply []runtime.Object

		// Create BMH and NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", "master", 6)
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		// Create improperly formatted BMC credential secret
		username := "root"
		password := "test"
		bmcSecret := testutil.CreateBMCAuthSecret(bmh.Name, bmh.Namespace, username, password)
		bmcSecret.Data = map[string][]byte{"foo": []byte("bad data!")}

		bmh.Spec.BMC.CredentialsName = bmcSecret.Name
		objsToApply = append(objsToApply, bmcSecret)

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			ReadyForScheduleCount: map[airshipv1.VMRole]int{
				airshipv1.VMControlPlane: 1,
				airshipv1.VMWorker:       0,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateBMCAuth(*sipCluster, k8sClient)).ToNot(BeNil())
	})

	It("Should not process a BMH when its Network Data secret is missing", func() {
		var objsToApply []runtime.Object

		// Create BMH and NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", "master", 6)
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		bmh.Spec.NetworkData.Name = "foo-does-not-exist"
		bmh.Spec.NetworkData.Namespace = "foo-does-not-exist"

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			ReadyForScheduleCount: map[airshipv1.VMRole]int{
				airshipv1.VMControlPlane: 1,
				airshipv1.VMWorker:       0,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateServiceAddresses(*sipCluster, k8sClient)).ToNot(BeNil())
	})

	It("Should not process a BMH when its Network Data secret is incorrectly formatted", func() {
		var objsToApply []runtime.Object

		// Create BMH and NetworkData secret
		bmh, networkData := testutil.CreateBMH(1, "default", "master", 6)
		objsToApply = append(objsToApply, bmh)
		objsToApply = append(objsToApply, networkData)

		networkData.Data = map[string][]byte{"foo": []byte("bad data!")}

		m, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).To(BeNil())

		ml := &MachineList{
			NamespacedName: types.NamespacedName{
				Name:      "vbmh",
				Namespace: "default",
			},
			Machines: map[string]*Machine{
				bmh.Name: m,
			},
			ReadyForScheduleCount: map[airshipv1.VMRole]int{
				airshipv1.VMControlPlane: 1,
				airshipv1.VMWorker:       0,
			},
			Log: ctrl.Log.WithName("controllers").WithName("SIPCluster"),
		}

		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		sipCluster.Spec.Services = airshipv1.SIPClusterServices{
			LoadBalancer: []airshipv1.SIPClusterService{
				{
					Image: "haproxy:latest",
					NodeLabels: map[string]string{
						"test": "true",
					},
					NodePort:      30000,
					NodeInterface: "oam-ipv4",
				},
			},
		}
		k8sClient := mockClient.NewFakeClient(objsToApply...)
		Expect(ml.ExtrapolateServiceAddresses(*sipCluster, k8sClient)).ToNot(BeNil())
	})

	It("Should not retrieve the BMH IP if it has been previously extrapolated", func() {
		// Store an IP address for each machine
		var objs []runtime.Object
		for _, machine := range machineList.Machines {
			machine.Data.IPOnInterface = map[string]string{
				"oam-ipv4": "32.68.51.139",
			}
			objs = append(objs, &machine.BMH)
		}

		k8sClient := mockClient.NewFakeClient(objs...)
		sipCluster := testutil.CreateSIPCluster("subcluster-1", "default", 1, 3)
		Expect(machineList.ExtrapolateServiceAddresses(*sipCluster, k8sClient)).To(BeNil())
	})

	It("Should not schedule BMH if it is missing networkdata", func() {
		// Create a BMH without NetworkData
		bmh, _ := testutil.CreateBMH(1, "default", airshipv1.VMControlPlane, 6)
		bmh.Spec.NetworkData = nil
		_, err := NewMachine(*bmh, airshipv1.VMControlPlane, NotScheduled)
		Expect(err).ToNot(BeNil())
	})
})
