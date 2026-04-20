package store

import (
	"github.com/teacat99/SubForge/internal/model"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func New(dbPath string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.Subscription{},
		&model.Node{},
		&model.ServiceGroup{},
		&model.ProfileService{},
		&model.ProfileNode{},
		&model.UserProfile{},
		&model.DnsPreset{},
		&model.HostsPreset{},
		&model.AdminUser{},
		&model.PublishedProfile{},
	); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) EnsureMigrate() {
	s.db.AutoMigrate(&model.Setting{})
}

// ── Admin ──

func (s *Store) EnsureAdmin(username, password string) error {
	var count int64
	s.db.Model(&model.AdminUser{}).Count(&count)
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Create(&model.AdminUser{Username: username, Password: string(hash)}).Error
}

func (s *Store) GetAdmin(username string) (*model.AdminUser, error) {
	var u model.AdminUser
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) UpdateAdminPassword(id uint, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).Update("password", string(hash)).Error
}

func (s *Store) UpdateAdminUsername(id uint, newUsername string) error {
	return s.db.Model(&model.AdminUser{}).Where("id = ?", id).Update("username", newUsername).Error
}

func (s *Store) GetFirstAdmin() (*model.AdminUser, error) {
	var u model.AdminUser
	if err := s.db.First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ── Subscription ──

func (s *Store) CreateSubscription(sub *model.Subscription) error {
	var maxOrder int
	s.db.Model(&model.Subscription{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	sub.SortOrder = maxOrder + 1
	return s.db.Create(sub).Error
}

func (s *Store) ListSubscriptions() ([]model.Subscription, error) {
	var subs []model.Subscription
	if err := s.db.Order("sort_order ASC, id ASC").Find(&subs).Error; err != nil {
		return nil, err
	}
	for i := range subs {
		var count int64
		s.db.Model(&model.Node{}).Where("subscription_id = ?", subs[i].ID).Count(&count)
		subs[i].NodeCount = int(count)
	}
	return subs, nil
}

func (s *Store) ReorderSubscriptions(ids []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&model.Subscription{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) GetSubscription(id uint) (*model.Subscription, error) {
	var sub model.Subscription
	if err := s.db.First(&sub, id).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) UpdateSubscription(sub *model.Subscription) error {
	return s.db.Save(sub).Error
}

func (s *Store) DeleteSubscription(id uint) error {
	s.db.Where("subscription_id = ?", id).Delete(&model.Node{})
	return s.db.Delete(&model.Subscription{}, id).Error
}

func (s *Store) UpdateSubscriptionFetched(id uint) error {
	return s.db.Model(&model.Subscription{}).Where("id = ?", id).
		Update("last_fetched", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

func (s *Store) UpdateSubscriptionTraffic(id uint, used, total int64, expiry string) error {
	return s.db.Model(&model.Subscription{}).Where("id = ?", id).
		Updates(map[string]any{"traffic_used": used, "traffic_total": total, "sub_expiry": expiry}).Error
}

// ── Node ──

func (s *Store) ReplaceNodes(subID uint, nodes []model.Node) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var oldIDs []uint
		tx.Model(&model.Node{}).Where("subscription_id = ?", subID).Pluck("id", &oldIDs)
		if len(oldIDs) > 0 {
			tx.Where("node_id IN ?", oldIDs).Delete(&model.ProfileNode{})
		}
		if err := tx.Where("subscription_id = ?", subID).Delete(&model.Node{}).Error; err != nil {
			return err
		}
		if len(nodes) > 0 {
			if err := tx.Create(&nodes).Error; err != nil {
				return err
			}
			var profiles []model.UserProfile
			tx.Find(&profiles)
			for _, p := range profiles {
				for _, n := range nodes {
					tx.Create(&model.ProfileNode{ProfileID: p.ID, NodeID: n.ID, Enabled: true})
				}
			}
		}
		return nil
	})
}

func (s *Store) ListNodesBySubscription(subID uint) ([]model.Node, error) {
	var nodes []model.Node
	return nodes, s.db.Where("subscription_id = ?", subID).Order("id ASC").Find(&nodes).Error
}

func (s *Store) ListNodes() ([]model.Node, error) {
	var nodes []model.Node
	return nodes, s.db.
		Joins("LEFT JOIN subscriptions ON subscriptions.id = nodes.subscription_id").
		Order("CASE WHEN nodes.subscription_id = 0 THEN 1 ELSE 0 END, subscriptions.sort_order ASC, nodes.id ASC").
		Find(&nodes).Error
}

func (s *Store) ListEnabledNodes() ([]model.Node, error) {
	var nodes []model.Node
	return nodes, s.db.Where("nodes.enabled = ?", true).
		Joins("LEFT JOIN subscriptions ON subscriptions.id = nodes.subscription_id").
		Order("CASE WHEN nodes.subscription_id = 0 THEN 1 ELSE 0 END, subscriptions.sort_order ASC, nodes.id ASC").
		Find(&nodes).Error
}

func (s *Store) ToggleNode(id uint, enabled bool) error {
	return s.db.Model(&model.Node{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (s *Store) UpdateNodeAlias(id uint, alias string) error {
	return s.db.Model(&model.Node{}).Where("id = ?", id).Update("alias", alias).Error
}

func (s *Store) GetNode(id uint) (*model.Node, error) {
	var n model.Node
	if err := s.db.First(&n, id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) CreateNode(node *model.Node) error {
	if err := s.db.Create(node).Error; err != nil {
		return err
	}
	var profiles []model.UserProfile
	s.db.Find(&profiles)
	for _, p := range profiles {
		s.db.Create(&model.ProfileNode{ProfileID: p.ID, NodeID: node.ID, Enabled: true})
	}
	return nil
}

func (s *Store) UpdateNode(node *model.Node) error {
	return s.db.Save(node).Error
}

func (s *Store) DeleteNode(id uint) error {
	s.db.Where("node_id = ?", id).Delete(&model.ProfileNode{})
	return s.db.Delete(&model.Node{}, id).Error
}

func (s *Store) UpdateNodeName(id uint, name string) error {
	return s.db.Model(&model.Node{}).Where("id = ?", id).Update("name", name).Error
}

// ── ServiceGroup ──

func (s *Store) SeedServiceGroups(groups []model.ServiceGroup) error {
	for i := range groups {
		var existing model.ServiceGroup
		if err := s.db.Where("name = ? AND builtin = ?", groups[i].Name, true).First(&existing).Error; err == nil {
			continue
		}
		if err := s.db.Create(&groups[i]).Error; err != nil {
			return err
		}
		s.db.Create(&model.ProfileService{
			ProfileID:      0,
			ServiceGroupID: groups[i].ID,
			Enabled:        true,
			SortOrder:      groups[i].SortOrder,
		})
	}
	return nil
}

func (s *Store) CreateServiceGroup(g *model.ServiceGroup) error {
	if err := s.db.Create(g).Error; err != nil {
		return err
	}
	var maxOrder int
	s.db.Model(&model.ProfileService{}).
		Where("profile_id = ?", 0).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	s.db.Create(&model.ProfileService{
		ProfileID: 0, ServiceGroupID: g.ID,
		Enabled: true, SortOrder: maxOrder + 1,
	})
	var profiles []model.UserProfile
	s.db.Find(&profiles)
	for _, p := range profiles {
		s.db.Create(&model.ProfileService{
			ProfileID: p.ID, ServiceGroupID: g.ID,
			Enabled: false, SortOrder: maxOrder + 1,
		})
	}
	return nil
}

func (s *Store) ListServiceGroups() ([]model.ServiceGroup, error) {
	var groups []model.ServiceGroup
	return groups, s.db.Order("sort_order ASC, id ASC").Find(&groups).Error
}

func (s *Store) GetServiceGroup(id uint) (*model.ServiceGroup, error) {
	var g model.ServiceGroup
	if err := s.db.First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) UpdateServiceGroup(g *model.ServiceGroup) error {
	return s.db.Save(g).Error
}

func (s *Store) ReorderServiceGroups(ids []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&model.ServiceGroup{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
			tx.Model(&model.ProfileService{}).
				Where("profile_id = 0 AND service_group_id = ?", id).
				Update("sort_order", i+1)
		}
		return nil
	})
}

func (s *Store) DeleteServiceGroup(id uint) error {
	s.db.Where("service_group_id = ?", id).Delete(&model.ProfileService{})
	return s.db.Delete(&model.ServiceGroup{}, id).Error
}

func (s *Store) GetServiceGroupsByIDs(ids []uint) ([]model.ServiceGroup, error) {
	var groups []model.ServiceGroup
	if len(ids) == 0 {
		return groups, nil
	}
	return groups, s.db.Where("id IN ?", ids).Find(&groups).Error
}

// ── ProfileService ──

func (s *Store) ListProfileServices(profileID uint) ([]model.ProfileService, error) {
	var ps []model.ProfileService
	return ps, s.db.Where("profile_id = ?", profileID).
		Order("sort_order ASC, id ASC").Find(&ps).Error
}

func (s *Store) CreateProfileService(ps *model.ProfileService) error {
	return s.db.Create(ps).Error
}

func (s *Store) ToggleProfileService(profileID, serviceGroupID uint, enabled bool) error {
	return s.db.Model(&model.ProfileService{}).
		Where("profile_id = ? AND service_group_id = ?", profileID, serviceGroupID).
		Update("enabled", enabled).Error
}

func (s *Store) UpdateProfileServiceProxy(profileID, serviceGroupID uint, proxy string) error {
	return s.db.Model(&model.ProfileService{}).
		Where("profile_id = ? AND service_group_id = ?", profileID, serviceGroupID).
		Update("default_proxy", proxy).Error
}

func (s *Store) ReorderProfileServices(profileID uint, ids []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&model.ProfileService{}).
				Where("profile_id = ? AND service_group_id = ?", profileID, id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ResetProfileServiceOrder(profileID uint) error {
	var groups []model.ServiceGroup
	s.db.Order("sort_order ASC, id ASC").Find(&groups)
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, g := range groups {
			tx.Model(&model.ProfileService{}).
				Where("profile_id = ? AND service_group_id = ?", profileID, g.ID).
				Updates(map[string]any{"sort_order": g.SortOrder, "enabled": g.Enabled})
		}
		return nil
	})
}

// SyncProfileServices reconciles a profile's ProfileService rows against the
// current global ServiceGroup set: it adds missing rows (inheriting global
// enabled/sort_order, default_proxy left empty for inheritance), and removes
// rows whose ServiceGroup no longer exists. Existing rows are kept untouched
// so user toggles / overrides / sort orders are preserved.
// Returns (added, removed, error).
func (s *Store) SyncProfileServices(profileID uint) (int, int, error) {
	added, removed := 0, 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var groups []model.ServiceGroup
		if err := tx.Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
			return err
		}
		groupSet := make(map[uint]model.ServiceGroup, len(groups))
		for _, g := range groups {
			groupSet[g.ID] = g
		}

		var existing []model.ProfileService
		if err := tx.Where("profile_id = ?", profileID).Find(&existing).Error; err != nil {
			return err
		}
		existingSet := make(map[uint]struct{}, len(existing))
		for _, ps := range existing {
			existingSet[ps.ServiceGroupID] = struct{}{}
		}

		for _, g := range groups {
			if _, ok := existingSet[g.ID]; ok {
				continue
			}
			ps := model.ProfileService{
				ProfileID:      profileID,
				ServiceGroupID: g.ID,
				Enabled:        g.Enabled,
				SortOrder:      g.SortOrder,
			}
			if err := tx.Create(&ps).Error; err != nil {
				return err
			}
			added++
		}

		for _, ps := range existing {
			if _, ok := groupSet[ps.ServiceGroupID]; ok {
				continue
			}
			if err := tx.Delete(&model.ProfileService{}, ps.ID).Error; err != nil {
				return err
			}
			removed++
		}
		return nil
	})
	return added, removed, err
}

func (s *Store) CopyProfileServices(fromProfileID, toProfileID uint) error {
	var src []model.ProfileService
	s.db.Where("profile_id = ?", fromProfileID).Find(&src)
	srcMap := make(map[uint]*model.ProfileService)
	for i := range src {
		srcMap[src[i].ServiceGroupID] = &src[i]
	}

	var allGroups []model.ServiceGroup
	s.db.Order("sort_order ASC, id ASC").Find(&allGroups)
	groupMap := make(map[uint]*model.ServiceGroup)
	for i := range allGroups {
		groupMap[allGroups[i].ID] = &allGroups[i]
	}

	for _, g := range allGroups {
		ps := model.ProfileService{
			ProfileID:      toProfileID,
			ServiceGroupID: g.ID,
			Enabled:        g.Enabled,
			SortOrder:      g.SortOrder,
		}
		if existing, ok := srcMap[g.ID]; ok {
			ps.Enabled = existing.Enabled
			ps.SortOrder = existing.SortOrder
		}
		s.db.Create(&ps)
	}
	return nil
}

// ── ProfileNode ──

func (s *Store) ListProfileNodes(profileID uint) ([]model.ProfileNode, error) {
	var pn []model.ProfileNode
	return pn, s.db.Where("profile_id = ?", profileID).Find(&pn).Error
}

func (s *Store) SetProfileNodes(profileID uint, nodeStates map[uint]bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for nodeID, enabled := range nodeStates {
			var existing model.ProfileNode
			err := tx.Where("profile_id = ? AND node_id = ?", profileID, nodeID).First(&existing).Error
			if err == nil {
				tx.Model(&existing).Update("enabled", enabled)
			} else {
				tx.Create(&model.ProfileNode{ProfileID: profileID, NodeID: nodeID, Enabled: enabled})
			}
		}
		return nil
	})
}

func (s *Store) CopyProfileNodes(toProfileID uint) error {
	var nodes []model.Node
	if err := s.db.Where("enabled = ?", true).Find(&nodes).Error; err != nil {
		return err
	}
	for _, n := range nodes {
		s.db.Create(&model.ProfileNode{ProfileID: toProfileID, NodeID: n.ID, Enabled: true})
	}
	return nil
}

func (s *Store) GetEnabledNodeIDsForProfile(profileID uint) ([]uint, error) {
	var pn []model.ProfileNode
	if err := s.db.Where("profile_id = ? AND enabled = ?", profileID, true).Find(&pn).Error; err != nil {
		return nil, err
	}
	if len(pn) == 0 {
		var ids []uint
		s.db.Model(&model.Node{}).Where("enabled = ?", true).Pluck("id", &ids)
		return ids, nil
	}
	ids := make([]uint, len(pn))
	for i, p := range pn {
		ids[i] = p.NodeID
	}
	return ids, nil
}

func (s *Store) GetNodesByIDs(ids []uint) ([]model.Node, error) {
	var nodes []model.Node
	if len(ids) == 0 {
		return nodes, nil
	}
	return nodes, s.db.Where("nodes.id IN ?", ids).
		Joins("LEFT JOIN subscriptions ON subscriptions.id = nodes.subscription_id").
		Order("CASE WHEN nodes.subscription_id = 0 THEN 1 ELSE 0 END, subscriptions.sort_order ASC, nodes.id ASC").
		Find(&nodes).Error
}

// ── Setting ──

func (s *Store) GetSetting(key string) (string, error) {
	var setting model.Setting
	if err := s.db.Where("`key` = ?", key).First(&setting).Error; err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (s *Store) SetSetting(key, value string) error {
	return s.db.Where("`key` = ?", key).Assign(model.Setting{Key: key, Value: value}).FirstOrCreate(&model.Setting{}).Error
}

// ── DnsPreset ──

func (s *Store) SeedDnsPresets(presets []model.DnsPreset) error {
	for i := range presets {
		var existing model.DnsPreset
		if err := s.db.Where("name = ? AND builtin = ?", presets[i].Name, true).First(&existing).Error; err == nil {
			continue
		}
		s.db.Create(&presets[i])
	}
	return nil
}

func (s *Store) ListDnsPresets() ([]model.DnsPreset, error) {
	var presets []model.DnsPreset
	return presets, s.db.Order("id ASC").Find(&presets).Error
}

func (s *Store) GetDnsPreset(id uint) (*model.DnsPreset, error) {
	var p model.DnsPreset
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateDnsPreset(p *model.DnsPreset) error {
	return s.db.Create(p).Error
}

func (s *Store) UpdateDnsPreset(p *model.DnsPreset) error {
	return s.db.Save(p).Error
}

func (s *Store) DeleteDnsPreset(id uint) error {
	return s.db.Delete(&model.DnsPreset{}, id).Error
}

// ── HostsPreset ──

func (s *Store) SeedHostsPresets(presets []model.HostsPreset) error {
	for i := range presets {
		var existing model.HostsPreset
		if err := s.db.Where("name = ? AND builtin = ?", presets[i].Name, true).First(&existing).Error; err == nil {
			continue
		}
		s.db.Create(&presets[i])
	}
	return nil
}

func (s *Store) ListHostsPresets() ([]model.HostsPreset, error) {
	var presets []model.HostsPreset
	return presets, s.db.Order("id ASC").Find(&presets).Error
}

func (s *Store) GetHostsPreset(id uint) (*model.HostsPreset, error) {
	var p model.HostsPreset
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateHostsPreset(p *model.HostsPreset) error {
	return s.db.Create(p).Error
}

func (s *Store) UpdateHostsPreset(p *model.HostsPreset) error {
	return s.db.Save(p).Error
}

func (s *Store) DeleteHostsPreset(id uint) error {
	return s.db.Delete(&model.HostsPreset{}, id).Error
}

// ── UserProfile ──

func (s *Store) CreateProfile(p *model.UserProfile) error {
	var maxOrder int
	s.db.Model(&model.UserProfile{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	p.SortOrder = maxOrder + 1
	if err := s.db.Create(p).Error; err != nil {
		return err
	}
	s.CopyProfileServices(0, p.ID)
	s.CopyProfileNodes(p.ID)
	return nil
}

func (s *Store) ListProfiles() ([]model.UserProfile, error) {
	var profiles []model.UserProfile
	return profiles, s.db.Order("sort_order ASC, id ASC").Find(&profiles).Error
}

func (s *Store) ReorderProfiles(ids []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&model.UserProfile{}).Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) GetProfileByToken(token string) (*model.UserProfile, error) {
	var p model.UserProfile
	if err := s.db.Where("token = ?", token).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetProfile(id uint) (*model.UserProfile, error) {
	var p model.UserProfile
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdateProfile(p *model.UserProfile) error {
	return s.db.Save(p).Error
}

func (s *Store) DeleteProfile(id uint) error {
	s.db.Where("profile_id = ?", id).Delete(&model.ProfileService{})
	s.db.Where("profile_id = ?", id).Delete(&model.ProfileNode{})
	return s.db.Delete(&model.UserProfile{}, id).Error
}

// ── PublishedProfile ──

func (s *Store) GetPublishedProfile(token string) (*model.PublishedProfile, error) {
	var p model.PublishedProfile
	if err := s.db.Where("token = ?", token).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpsertPublishedProfile(token, config string) (*model.PublishedProfile, error) {
	var existing model.PublishedProfile
	if err := s.db.Where("token = ?", token).First(&existing).Error; err == nil {
		existing.Config = config
		existing.UpdatedAt = time.Now()
		return &existing, s.db.Save(&existing).Error
	}
	p := model.PublishedProfile{Token: token, Config: config, UpdatedAt: time.Now()}
	return &p, s.db.Create(&p).Error
}

// ── Import / Export ──

func (s *Store) ListSettings() ([]model.Setting, error) {
	var settings []model.Setting
	return settings, s.db.Find(&settings).Error
}

func (s *Store) ListAllProfileNodes() ([]model.ProfileNode, error) {
	var pn []model.ProfileNode
	return pn, s.db.Find(&pn).Error
}

func (s *Store) ListAllProfileServices() ([]model.ProfileService, error) {
	var ps []model.ProfileService
	return ps, s.db.Find(&ps).Error
}

func (s *Store) ListNodesWithRaw() ([]model.Node, error) {
	var nodes []model.Node
	return nodes, s.db.Order("id ASC").Find(&nodes).Error
}

func (s *Store) ImportData(
	subs []model.Subscription,
	nodes []model.Node,
	groups []model.ServiceGroup,
	profiles []model.UserProfile,
	profileNodes []model.ProfileNode,
	profileServices []model.ProfileService,
	dnsPresets []model.DnsPreset,
	hostsPresets []model.HostsPreset,
	settings []model.Setting,
) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("1=1").Delete(&model.ProfileService{})
		tx.Where("1=1").Delete(&model.ProfileNode{})
		tx.Where("1=1").Delete(&model.UserProfile{})
		tx.Where("1=1").Delete(&model.Node{})
		tx.Where("1=1").Delete(&model.Subscription{})
		tx.Where("1=1").Delete(&model.ServiceGroup{})
		tx.Where("1=1").Delete(&model.DnsPreset{})
		tx.Where("1=1").Delete(&model.HostsPreset{})
		tx.Where("1=1").Delete(&model.Setting{})

		if len(subs) > 0 {
			if err := tx.Create(&subs).Error; err != nil {
				return err
			}
		}
		if len(nodes) > 0 {
			if err := tx.Create(&nodes).Error; err != nil {
				return err
			}
		}
		if len(groups) > 0 {
			if err := tx.Create(&groups).Error; err != nil {
				return err
			}
		}
		if len(profiles) > 0 {
			if err := tx.Create(&profiles).Error; err != nil {
				return err
			}
		}
		if len(profileNodes) > 0 {
			if err := tx.Create(&profileNodes).Error; err != nil {
				return err
			}
		}
		if len(profileServices) > 0 {
			if err := tx.Create(&profileServices).Error; err != nil {
				return err
			}
		}
		if len(dnsPresets) > 0 {
			if err := tx.Create(&dnsPresets).Error; err != nil {
				return err
			}
		}
		if len(hostsPresets) > 0 {
			if err := tx.Create(&hostsPresets).Error; err != nil {
				return err
			}
		}
		if len(settings) > 0 {
			if err := tx.Create(&settings).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
